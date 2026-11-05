package mqttload

import (
	"fmt"

	"context"
	"errors"
	"github.com/HdrHistogram/hdrhistogram-go"
	"strings"
	"testing"
	"time"
)

// TestRunSubscribe_RecordsDeliveredMessagesInProgress checks that messages delivered through
// the fake client reach progress, and that progress and the client are both marked done/
// disconnected once ctx is cancelled.
func TestRunSubscribe_RecordsDeliveredMessagesInProgress(t *testing.T) {
	client := &fakeClient{}
	progress := NewSubscribeProgress()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunSubscribe(ctx, client, discardLogger(), "load/test", 1, progress, NewSubscribeStats(false), 0)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	const messageCount = 5
	for i := 0; i < messageCount; i++ {
		client.deliver("load/test", []byte("payload"))
	}

	waitFor(t, time.Second, func() bool { return progress.Snapshot().Received == messageCount })

	if progress.Snapshot().SubscribedAt.IsZero() {
		t.Error("expected SubscribedAt to be set once the subscribe was acknowledged")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunSubscribe did not return after ctx was cancelled")
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}

	select {
	case <-progress.Done():
	default:
		t.Error("expected progress to be marked done")
	}
}

func TestRunSubscribe_SubscribeUsesRequestedTopicAndQoS(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx already cancelled: RunSubscribe returns immediately after subscribing

	progress := NewSubscribeProgress()
	err := RunSubscribe(ctx, client, discardLogger(), "load/mytopic", 2, progress, NewSubscribeStats(false), 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	subscriptions := client.subscribeCalls()
	if len(subscriptions) != 1 {
		t.Fatalf("expected 1 subscribe call, got %d", len(subscriptions))
	}
	if subscriptions[0].topic != "load/mytopic" {
		t.Errorf("expected topic %q, got %q", "load/mytopic", subscriptions[0].topic)
	}
	if subscriptions[0].qos != 2 {
		t.Errorf("expected QoS 2, got %d", subscriptions[0].qos)
	}
}

func TestRunSubscribe_CancelledContextStopsAndDisconnects(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	progress := NewSubscribeProgress()
	err := RunSubscribe(ctx, client, discardLogger(), "load/test", 1, progress, NewSubscribeStats(false), 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

func TestRunSubscribe_FailedSubscribeReturnsError(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: true, err: errors.New("boom")}}
	progress := NewSubscribeProgress()

	err := RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress, NewSubscribeStats(false), 0)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to wrap the subscribe failure, got: %v", err)
	}

	// The display waits for Done before the command can return: if it never closes, sub hangs.
	select {
	case <-progress.Done():
	default:
		t.Error("expected progress to be finished after a failed subscribe")
	}
	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called after a failed subscribe")
	}
	if !progress.Snapshot().SubscribedAt.IsZero() {
		t.Error("expected SubscribedAt to stay zero when the subscribe fails")
	}
}

func TestRunSubscribe_SubscribeTimesOut(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: false}}
	progress := NewSubscribeProgress()

	err := RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress, NewSubscribeStats(false), 0)
	if err == nil {
		t.Fatal("expected a timeout error, got none")
	}

	// The display waits for Done before the command can return: if it never closes, sub hangs.
	select {
	case <-progress.Done():
	default:
		t.Error("expected progress to be finished after a failed subscribe")
	}
	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called after a failed subscribe")
	}
	if !progress.Snapshot().SubscribedAt.IsZero() {
		t.Error("expected SubscribedAt to stay zero when the subscribe times out")
	}
}

// TestRunSubscribe_StopsAfterCount checks that RunSubscribe returns once count messages have
// arrived, without ctx being cancelled.
func TestRunSubscribe_StopsAfterCount(t *testing.T) {
	client := &fakeClient{}
	progress := NewSubscribeProgress()

	done := make(chan error, 1)
	go func() {
		done <- RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress, NewSubscribeStats(false), 3)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	for i := 0; i < 3; i++ {
		client.deliver("load/test", []byte("payload"))
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunSubscribe did not return after the third message")
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunSubscribe_BenchmarkLatency checks that with benchmark on, the three kinds of payload
// land in the latency set, the without-timestamp count and the negative-latency count.
func TestRunSubscribe_BenchmarkLatency(t *testing.T) {
	client := &fakeClient{}
	progress := NewSubscribeProgress()
	stats := NewSubscribeStats(true)

	done := make(chan error, 1)
	go func() {
		done <- RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress, stats, 3)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	client.deliver("load/test", newPayloadGenerator(100, true)())
	client.deliver("load/test", []byte("no timestamp here"))
	client.deliver("load/test", fmt.Appendf(nil, `{"timestamp":%d,"padding":""}`, time.Now().Add(time.Hour).UnixMilli()))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunSubscribe did not return after the third message")
	}

	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)
	stats.latency.addTo(histogram)

	if histogram.TotalCount() != 1 {
		t.Errorf("expected 1 recorded latency, got %d", histogram.TotalCount())
	}
	if stats.withoutTimestamp.Load() != 1 {
		t.Errorf("expected 1 message without timestamp, got %d", stats.withoutTimestamp.Load())
	}
	if stats.negativeLatency.Load() != 1 {
		t.Errorf("expected 1 negative latency, got %d", stats.negativeLatency.Load())
	}
}

// TestRunSubscribe_NoLatencyWithoutBenchmark checks that nothing is recorded when benchmark
// is off.
func TestRunSubscribe_NoLatencyWithoutBenchmark(t *testing.T) {
	client := &fakeClient{}
	progress := NewSubscribeProgress()
	stats := NewSubscribeStats(false)

	done := make(chan error, 1)
	go func() {
		done <- RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress, stats, 2)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	client.deliver("load/test", newPayloadGenerator(100, true)())
	client.deliver("load/test", []byte("no timestamp here"))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunSubscribe did not return after the second message")
	}

	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)
	stats.latency.addTo(histogram)

	if histogram.TotalCount() != 0 || stats.withoutTimestamp.Load() != 0 || stats.negativeLatency.Load() != 0 {
		t.Errorf("expected nothing recorded, got %d latencies, %d without timestamp, %d negative",
			histogram.TotalCount(), stats.withoutTimestamp.Load(), stats.negativeLatency.Load())
	}
}
