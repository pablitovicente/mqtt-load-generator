package mqttload

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

func TestClientTopic(t *testing.T) {
	tests := []struct {
		topic        string
		clientNumber int
		suffix       bool
		want         string
	}{
		{"load/test", 1, false, "load/test"},
		{"load/test", 5, false, "load/test"},
		{"load/test", 1, true, "load/test/1"},
		{"load/test", 42, true, "load/test/42"},
	}

	for _, tt := range tests {
		got := clientTopic(tt.topic, tt.clientNumber, tt.suffix)
		if got != tt.want {
			t.Errorf("clientTopic(%q, %d, %v) = %q, want %q", tt.topic, tt.clientNumber, tt.suffix, got, tt.want)
		}
	}
}

// TestRunClientPublish_NeverMoreThanInFlightOutstanding checks the in-flight window: with fake
// tokens that stay pending until released, no more than InFlight publishes should ever be
// outstanding at once.
func TestRunClientPublish_NeverMoreThanInFlightOutstanding(t *testing.T) {
	const inFlight = 2
	const count = 5

	client := &fakePublisher{}

	var mutex sync.Mutex
	var tokens []*pendingToken

	client.nextToken = func(_ fakePublishCall) broker.Token {
		token := newPendingToken()
		mutex.Lock()
		tokens = append(tokens, token)
		mutex.Unlock()
		return token
	}

	tokenCount := func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return len(tokens)
	}

	options := PublishOptions{
		Topic: "load/test", QoS: 1, Count: count, Size: 10,
		Schedule: "flat", InFlight: inFlight, AckTimeout: time.Second,
	}
	counters := &publishCounters{}

	done := make(chan struct{})
	go func() {
		runClientPublish(context.Background(), client, 1, options, counters)
		close(done)
	}()

	// The run should stall once inFlight tokens are outstanding and none have been released.
	waitFor(t, time.Second, func() bool { return tokenCount() == inFlight })
	time.Sleep(20 * time.Millisecond)
	if got := tokenCount(); got != inFlight {
		t.Fatalf("expected exactly %d outstanding publishes, got %d", inFlight, got)
	}

	// Release them one at a time; each release should let exactly one more publish through,
	// until Count is reached.
	for i := 0; i < count; i++ {
		mutex.Lock()
		token := tokens[i]
		mutex.Unlock()

		token.release(nil)

		if i+1 < count {
			want := i + 1 + inFlight
			if want > count {
				want = count
			}
			waitFor(t, time.Second, func() bool { return tokenCount() == want })
		}
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runClientPublish did not return")
	}

	if got := len(client.publishCalls()); got != count {
		t.Errorf("expected %d publishes total, got %d", count, got)
	}
	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunClientPublish_CountsAckedFailedAndTimedOut checks that the counters end up right with
// one publish that succeeds, one that fails, and one that hangs past the ack timeout.
func TestRunClientPublish_CountsAckedFailedAndTimedOut(t *testing.T) {
	client := &fakePublisher{}

	var callIndex atomic.Int32
	client.nextToken = func(_ fakePublishCall) broker.Token {
		switch callIndex.Add(1) {
		case 1:
			return &fakeToken{completed: true} // acked
		case 2:
			return &fakeToken{completed: true, err: errors.New("boom")} // failed
		default:
			return newPendingToken() // never released: times out
		}
	}

	options := PublishOptions{
		Topic: "load/test", QoS: 1, Count: 3, Size: 10,
		Schedule: "flat", InFlight: 3, AckTimeout: 20 * time.Millisecond,
	}
	counters := &publishCounters{}

	done := make(chan struct{})
	go func() {
		runClientPublish(context.Background(), client, 1, options, counters)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runClientPublish did not return")
	}

	summary := sumPublishCounters([]*publishCounters{counters})
	if summary.Published != 3 {
		t.Errorf("Published = %d, want 3", summary.Published)
	}
	if summary.Acked != 1 {
		t.Errorf("Acked = %d, want 1", summary.Acked)
	}
	if summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", summary.Failed)
	}
	if summary.TimedOut != 1 {
		t.Errorf("TimedOut = %d, want 1", summary.TimedOut)
	}
	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunPublish_TopicQoSAndSuffix checks that RunPublish sends the right number of messages at
// the right QoS, and that --suffix gives each client its own numbered sub-topic, 1..N.
func TestRunPublish_TopicQoSAndSuffix(t *testing.T) {
	const clientCount = 3
	const count = 2

	var mutex sync.Mutex
	var fakes []*fakePublisher

	connect := func(_ context.Context, _ int) (Publisher, error) {
		client := &fakePublisher{}
		mutex.Lock()
		fakes = append(fakes, client)
		mutex.Unlock()
		return client, nil
	}

	options := PublishOptions{
		Topic: "load/test", QoS: 2, Count: count, Size: 8,
		Schedule: "flat", Clients: clientCount, Suffix: true,
		InFlight: 1, AckTimeout: time.Second, ConnectConcurrency: clientCount,
	}

	if err := RunPublish(context.Background(), connect, discardLogger(), options, io.Discard); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(fakes) != clientCount {
		t.Fatalf("expected %d clients connected, got %d", clientCount, len(fakes))
	}

	seenTopics := map[string]bool{}
	for i, client := range fakes {
		calls := client.publishCalls()
		if len(calls) != count {
			t.Errorf("client %d: expected %d publishes, got %d", i, count, len(calls))
		}
		for _, call := range calls {
			if call.qos != 2 {
				t.Errorf("client %d: qos = %d, want 2", i, call.qos)
			}
			seenTopics[call.topic] = true
		}
		if !client.wasDisconnected() {
			t.Errorf("client %d: expected Disconnect to be called", i)
		}
	}

	wantTopics := []string{"load/test/1", "load/test/2", "load/test/3"}
	if len(seenTopics) != len(wantTopics) {
		t.Fatalf("topics published to = %v, want %v", seenTopics, wantTopics)
	}
	for _, topic := range wantTopics {
		if !seenTopics[topic] {
			t.Errorf("expected topic %q to have been published to", topic)
		}
	}
}

// TestRunPublish_CtxCancelStopsEarlyAndDrains checks that cancelling ctx mid-run stops sending
// new messages, still waits for whatever was already in flight, disconnects, and returns nil
// since nothing failed or timed out.
func TestRunPublish_CtxCancelStopsEarlyAndDrains(t *testing.T) {
	client := &fakePublisher{}

	client.nextToken = func(_ fakePublishCall) broker.Token {
		token := newPendingToken()
		// Release shortly after being created, so the post-cancel drain finishes quickly
		// instead of running into AckTimeout.
		go func() {
			time.Sleep(5 * time.Millisecond)
			token.release(nil)
		}()
		return token
	}

	connect := func(_ context.Context, _ int) (Publisher, error) { return client, nil }

	const count = 1000
	options := PublishOptions{
		Topic: "load/test", QoS: 0, Count: count, Size: 8,
		Schedule: "flat", IntervalMilliseconds: 5, Clients: 1,
		InFlight: 4, AckTimeout: time.Second, ConnectConcurrency: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunPublish(ctx, connect, discardLogger(), options, io.Discard)
	}()

	waitFor(t, time.Second, func() bool { return len(client.publishCalls()) >= 3 })
	cancel()

	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPublish did not return after ctx was cancelled")
	}

	if err != nil {
		t.Fatalf("expected no error (no failures or timeouts), got %v", err)
	}

	published := len(client.publishCalls())
	if published == 0 || published >= count {
		t.Errorf("expected the run to stop early, published %d of %d", published, count)
	}
	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunPublish_ExitCode checks RunPublish's exit behaviour: nil on a clean run, an error
// naming the counts when there are failures or timeouts.
func TestRunPublish_ExitCode(t *testing.T) {
	baseOptions := PublishOptions{
		Topic: "load/test", QoS: 0, Size: 4, Schedule: "flat",
		Clients: 1, InFlight: 2, ConnectConcurrency: 1,
	}

	t.Run("clean run returns nil", func(t *testing.T) {
		client := &fakePublisher{}
		connect := func(_ context.Context, _ int) (Publisher, error) { return client, nil }

		options := baseOptions
		options.Count = 5
		options.AckTimeout = time.Second

		if err := RunPublish(context.Background(), connect, discardLogger(), options, io.Discard); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("failures produce an error mentioning the counts", func(t *testing.T) {
		client := &fakePublisher{nextToken: func(_ fakePublishCall) broker.Token {
			return &fakeToken{completed: true, err: errors.New("boom")}
		}}
		connect := func(_ context.Context, _ int) (Publisher, error) { return client, nil }

		options := baseOptions
		options.Count = 3
		options.AckTimeout = time.Second

		err := RunPublish(context.Background(), connect, discardLogger(), options, io.Discard)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "3 published") || !strings.Contains(err.Error(), "3 failed") {
			t.Errorf("expected error to mention the counts, got: %v", err)
		}
	})

	t.Run("timeouts produce an error mentioning the counts", func(t *testing.T) {
		client := &fakePublisher{nextToken: func(_ fakePublishCall) broker.Token {
			return newPendingToken() // never released
		}}
		connect := func(_ context.Context, _ int) (Publisher, error) { return client, nil }

		options := baseOptions
		options.Count = 2
		options.AckTimeout = 10 * time.Millisecond

		err := RunPublish(context.Background(), connect, discardLogger(), options, io.Discard)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "2 timed out") {
			t.Errorf("expected error to mention the timed out count, got: %v", err)
		}
	})
}

// TestRunPublish_ConnectFailureReturnsError checks that a connect failure comes back as an
// error instead of publishing anything.
func TestRunPublish_ConnectFailureReturnsError(t *testing.T) {
	wantErr := errors.New("dial failed")
	connect := func(_ context.Context, _ int) (Publisher, error) { return nil, wantErr }

	options := PublishOptions{
		Topic: "load/test", Count: 1, Size: 4, Schedule: "flat",
		Clients: 1, InFlight: 1, AckTimeout: time.Second, ConnectConcurrency: 1,
	}

	err := RunPublish(context.Background(), connect, discardLogger(), options, io.Discard)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
}

// TestRunPublish_CancelDuringConnectIsNotAnError checks that Ctrl-C while clients are still
// connecting ends the run cleanly (exit 0), the same as Ctrl-C while publishing.
func TestRunPublish_CancelDuringConnectIsNotAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// The connect "hangs" until ctx is cancelled, like a slow broker when the user presses
	// Ctrl-C.
	connect := func(connectCtx context.Context, _ int) (Publisher, error) {
		cancel()
		<-connectCtx.Done()
		return nil, connectCtx.Err()
	}

	options := PublishOptions{
		Topic: "load/test", Count: 1, Size: 4, Schedule: "flat",
		Clients: 1, InFlight: 1, AckTimeout: time.Second, ConnectConcurrency: 1,
	}

	if err := RunPublish(ctx, connect, discardLogger(), options, io.Discard); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
