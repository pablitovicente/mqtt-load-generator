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
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, DumpOptions{}, &output)
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
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, DumpOptions{}, &output)
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

	err := RunDump(ctx, client, discardLogger(), "load/mytopic", 2, DumpOptions{}, io.Discard)
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
	err := RunDump(ctx, client, discardLogger(), "load/test", 1, DumpOptions{}, &output)
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

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, DumpOptions{}, io.Discard)
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

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, DumpOptions{}, io.Discard)
	if err == nil {
		t.Fatal("expected a timeout error, got none")
	}
}

func TestFormatLine(t *testing.T) {
	tests := []struct {
		name      string
		topic     string
		payload   string
		showTopic bool
		want      string
	}{
		{"payload only", "load/test", `{"timestamp":1}`, false, "{\"timestamp\":1}\n"},
		{"topic and payload", "load/test", `{"timestamp":1}`, true, "load/test\t{\"timestamp\":1}\n"},
		{"topic with a space", "load/my topic", "x", true, "load/my topic\tx\n"},
		{"empty payload", "load/test", "", false, "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(formatLine(tt.topic, []byte(tt.payload), tt.showTopic))
			if got != tt.want {
				t.Errorf("formatLine = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunDump_ShowTopicPrintsTopicAndTab checks that --show-topic reaches the output lines.
func TestRunDump_ShowTopicPrintsTopicAndTab(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, DumpOptions{ShowTopic: true}, &output)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	client.deliver("load/test", []byte("payload"))
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if output.String() != "load/test\tpayload\n" {
		t.Errorf("output = %q, want %q", output.String(), "load/test\tpayload\n")
	}
}

// failingWriter is an io.Writer that always fails, like stdout on a full disk.
type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("disk full")
}

// TestRunDump_WriteErrorStopsAndReturnsError checks that a broken output ends the run with the
// write error, and still disconnects, instead of carrying on silently until Ctrl-C.
func TestRunDump_WriteErrorStopsAndReturnsError(t *testing.T) {
	client := &fakeClient{}

	done := make(chan error, 1)
	go func() {
		done <- RunDump(context.Background(), client, discardLogger(), "load/test", 1, DumpOptions{}, failingWriter{})
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	// A second failing write must not block: only the first error is reported.
	client.deliver("load/test", []byte("first"))
	client.deliver("load/test", []byte("second"))

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("expected the write error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunDump did not stop after a write error")
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}
