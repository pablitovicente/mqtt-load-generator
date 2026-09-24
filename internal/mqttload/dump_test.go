package mqttload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRunDump_WritesPayloadsAsLines checks that payloads delivered through the fake client
// reach output as text, one per line, in delivery order, and that nothing else is written to
// output (RunDump has no "press ctrl+c" style line the way sub does).
func TestRunDump_WritesPayloadsAsLines(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, &output)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	client.deliver("load/test", []byte("first"))
	client.deliver("load/test", []byte("second"))

	waitFor(t, time.Second, func() bool { return strings.Count(output.String(), "\n") == 2 })

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunDump did not return after ctx was cancelled")
	}

	want := "first\nsecond\n"
	if output.String() != want {
		t.Errorf("output = %q, want %q", output.String(), want)
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunDump_ConcurrentDeliveryProducesWholeLines delivers many payloads from many goroutines
// at once, the way paho's own callback goroutines would, and checks that every line in output
// is exactly one of the payloads sent -- never a mix of two. Run with -race: output is a plain
// bytes.Buffer, not safe for concurrent use on its own, so a missing or broken lock in RunDump
// would show up either as a reported data race or as a corrupted line.
func TestRunDump_ConcurrentDeliveryProducesWholeLines(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, &output)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	const goroutineCount = 100

	payloads := make([]string, goroutineCount)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("payload-%03d-with-extra-padding-so-a-partial-write-would-show", i)
	}

	var wg sync.WaitGroup
	for _, payload := range payloads {
		wg.Add(1)
		go func(payload string) {
			defer wg.Done()
			client.deliver("load/test", []byte(payload))
		}(payload)
	}
	wg.Wait() // every deliver() call, and so every writeLine call it made, has returned by here

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	trimmed := strings.TrimRight(output.String(), "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) != goroutineCount {
		t.Fatalf("expected %d lines, got %d: %q", goroutineCount, len(lines), output.String())
	}

	want := make(map[string]bool, goroutineCount)
	for _, payload := range payloads {
		want[payload] = true
	}
	for _, line := range lines {
		if !want[line] {
			t.Errorf("unexpected or corrupted line: %q", line)
		}
	}
}

// TestRunDump_SubscribeUsesRequestedTopicAndQoS checks that RunDump subscribes with the topic
// and QoS it was given.
func TestRunDump_SubscribeUsesRequestedTopicAndQoS(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx already cancelled: RunDump returns immediately after subscribing

	err := RunDump(ctx, client, discardLogger(), "load/mytopic", 2, io.Discard)
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

// TestRunDump_CancelledContextStopsAndDisconnects checks that RunDump returns nil and
// disconnects once its context is cancelled.
func TestRunDump_CancelledContextStopsAndDisconnects(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	err := RunDump(ctx, client, discardLogger(), "load/test", 1, &output)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunDump_FailedSubscribeReturnsError checks that a subscribe failure comes back as an
// error instead of hanging or panicking.
func TestRunDump_FailedSubscribeReturnsError(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: true, err: errors.New("boom")}}

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, io.Discard)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to wrap the subscribe failure, got: %v", err)
	}
}

// TestRunDump_SubscribeTimesOut checks that a subscribe that never gets acknowledged comes back
// as an error instead of hanging forever.
func TestRunDump_SubscribeTimesOut(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: false}}

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, io.Discard)
	if err == nil {
		t.Fatal("expected a timeout error, got none")
	}
}
