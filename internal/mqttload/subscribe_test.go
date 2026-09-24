package mqttload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// discardLogger returns a logger that throws away everything, for tests that don't care about
// log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// syncBuffer is a bytes.Buffer safe for one goroutine to write to (the reporting loop, via the
// logger) while the test goroutine reads it, so tests can run under -race.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(p)
}

func (b *syncBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}

// captureLogger returns a logger and the buffer its output lands in, as plain text so tests
// can check for a substring without decoding JSON.
func captureLogger() (*slog.Logger, *syncBuffer) {
	buffer := &syncBuffer{}
	return slog.New(slog.NewTextHandler(buffer, nil)), buffer
}

// waitFor polls condition until it returns true or the timeout passes, failing the test if it
// never does. It exists so tests don't have to guess a fixed sleep long enough for a
// background goroutine to catch up.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", timeout)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRunSubscribe_CountsDeliveredMessages checks that messages delivered through the fake client
// reach the counter, by reading them back from a --disable-bar log line: runSubscribe's counter is
// unexported, so the log output is the observable proof that the subscribe callback ran.
func TestRunSubscribe_CountsDeliveredMessages(t *testing.T) {
	client := &fakeClient{}
	logger, output := captureLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runSubscribe(ctx, client, logger, "load/test", 1, SubscribeOptions{DisableBar: true, ResetAfter: time.Hour}, io.Discard, time.Millisecond, 5*time.Millisecond)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	const messageCount = 5
	for i := 0; i < messageCount; i++ {
		client.deliver("load/test", []byte("payload"))
	}

	waitFor(t, time.Second, func() bool {
		return strings.Contains(output.String(), "received 5 messages so far")
	})

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runSubscribe did not return after ctx was cancelled")
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

func TestRunSubscribe_SubscribeUsesRequestedTopicAndQoS(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx already cancelled: the loop returns immediately after subscribing

	err := runSubscribe(ctx, client, discardLogger(), "load/mytopic", 2, SubscribeOptions{DisableBar: true, ResetAfter: time.Second}, io.Discard, time.Millisecond, time.Millisecond)
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

func TestRunSubscribe_CancelledContextStopsLoopAndDisconnects(t *testing.T) {
	tests := []struct {
		name       string
		disableBar bool
	}{
		{"progress bar mode", false},
		{"disable-bar mode", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			var output bytes.Buffer
			err := runSubscribe(ctx, client, discardLogger(), "load/test", 1, SubscribeOptions{DisableBar: tt.disableBar, ResetAfter: time.Second}, &output, time.Millisecond, time.Millisecond)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if !client.wasDisconnected() {
				t.Error("expected Disconnect to be called")
			}
		})
	}
}

func TestRunSubscribe_FailedSubscribeReturnsError(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: true, err: errors.New("boom")}}

	err := runSubscribe(context.Background(), client, discardLogger(), "load/test", 1, SubscribeOptions{DisableBar: true, ResetAfter: time.Second}, io.Discard, time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to wrap the subscribe failure, got: %v", err)
	}
}

func TestRunSubscribe_SubscribeTimesOut(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: false}}

	err := runSubscribe(context.Background(), client, discardLogger(), "load/test", 1, SubscribeOptions{DisableBar: true, ResetAfter: time.Second}, io.Discard, time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got none")
	}
}

func TestBuildRateReport(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		previousCount   int64
		currentCount    int64
		lastMessageAt   time.Time
		resetAfter      time.Duration
		wantLineEmpty   bool
		wantShouldReset bool
	}{
		{
			name:          "zero count logs nothing",
			previousCount: 0,
			currentCount:  0,
			lastMessageAt: now,
			resetAfter:    time.Second,
			wantLineEmpty: true,
		},
		{
			name:          "positive count logs a line",
			previousCount: 10,
			currentCount:  15,
			lastMessageAt: now,
			resetAfter:    time.Minute,
			wantLineEmpty: false,
		},
		{
			name:            "stale counter logs a line and resets",
			previousCount:   10,
			currentCount:    10,
			lastMessageAt:   now.Add(-2 * time.Second),
			resetAfter:      time.Second,
			wantLineEmpty:   false,
			wantShouldReset: true,
		},
		{
			name:          "exactly at the reset boundary does not reset",
			previousCount: 10,
			currentCount:  10,
			lastMessageAt: now.Add(-time.Second),
			resetAfter:    time.Second,
			wantLineEmpty: false,
		},
		{
			name:          "zero-value lastMessageAt never triggers a reset",
			previousCount: 0,
			currentCount:  3,
			lastMessageAt: time.Time{},
			resetAfter:    time.Nanosecond,
			wantLineEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildRateReport(tt.previousCount, tt.currentCount, tt.lastMessageAt, now, tt.resetAfter)

			if (report.line == "") != tt.wantLineEmpty {
				t.Errorf("line = %q, wantEmpty %v", report.line, tt.wantLineEmpty)
			}
			if report.shouldReset != tt.wantShouldReset {
				t.Errorf("shouldReset = %v, want %v", report.shouldReset, tt.wantShouldReset)
			}
		})
	}
}

func TestMessageCounter(t *testing.T) {
	var counter messageCounter

	count, lastMessageAt := counter.snapshot()
	if count != 0 || !lastMessageAt.IsZero() {
		t.Fatalf("expected a zero-value counter, got count=%d lastMessageAt=%v", count, lastMessageAt)
	}

	counter.recordMessage()
	counter.recordMessage()

	count, lastMessageAt = counter.snapshot()
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
	if lastMessageAt.IsZero() {
		t.Error("expected lastMessageAt to be set after recordMessage")
	}

	counter.reset()
	count, _ = counter.snapshot()
	if count != 0 {
		t.Errorf("expected count 0 after reset, got %d", count)
	}
}
