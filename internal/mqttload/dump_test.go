package mqttload

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestRunDump_CallsPrinterForEachMessage checks that payloads delivered through the fake client
// reach the printer, in delivery order. Formatting itself is display's concern (see
// display.TestMessagePrinter*).
func TestRunDump_CallsPrinterForEachMessage(t *testing.T) {
	client := &fakeClient{}
	printer := &fakePrinter{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunDump(ctx, client, discardLogger(), "load/test", 1, printer)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	client.deliver("load/test", []byte("first"))
	client.deliver("load/test", []byte("second"))

	waitFor(t, time.Second, func() bool { return len(printer.printCalls()) == 2 })

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunDump did not return after ctx was cancelled")
	}

	calls := printer.printCalls()
	if string(calls[0].payload) != "first" || string(calls[1].payload) != "second" {
		t.Errorf("unexpected calls: %+v", calls)
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}

// TestRunDump_SubscribeUsesRequestedTopicAndQoS checks that RunDump subscribes with the topic
// and QoS it was given.
func TestRunDump_SubscribeUsesRequestedTopicAndQoS(t *testing.T) {
	client := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx already cancelled: RunDump returns immediately after subscribing

	err := RunDump(ctx, client, discardLogger(), "load/mytopic", 2, &fakePrinter{})
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

	err := RunDump(ctx, client, discardLogger(), "load/test", 1, &fakePrinter{})
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

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, &fakePrinter{})
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

	err := RunDump(context.Background(), client, discardLogger(), "load/test", 1, &fakePrinter{})
	if err == nil {
		t.Fatal("expected a timeout error, got none")
	}
}

// TestRunDump_PrintErrorStopsAndReturnsError checks that a printer reporting an error ends the
// run with that error, and still disconnects, instead of carrying on silently until Ctrl-C.
func TestRunDump_PrintErrorStopsAndReturnsError(t *testing.T) {
	client := &fakeClient{}
	printer := &fakePrinter{err: func(_ string, _ []byte) error { return errors.New("disk full") }}

	done := make(chan error, 1)
	go func() {
		done <- RunDump(context.Background(), client, discardLogger(), "load/test", 1, printer)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	// A second failing print must not block: only the first error is reported.
	client.deliver("load/test", []byte("first"))
	client.deliver("load/test", []byte("second"))

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("expected the print error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunDump did not stop after a print error")
	}

	if !client.wasDisconnected() {
		t.Error("expected Disconnect to be called")
	}
}
