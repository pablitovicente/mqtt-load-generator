package mqttload

import (
	"context"
	"errors"
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
		done <- RunSubscribe(ctx, client, discardLogger(), "load/test", 1, progress)
	}()

	waitFor(t, time.Second, func() bool { return len(client.subscribeCalls()) == 1 })

	const messageCount = 5
	for i := 0; i < messageCount; i++ {
		client.deliver("load/test", []byte("payload"))
	}

	waitFor(t, time.Second, func() bool { return progress.Snapshot().Received == messageCount })

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
	err := RunSubscribe(ctx, client, discardLogger(), "load/mytopic", 2, progress)
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
	err := RunSubscribe(ctx, client, discardLogger(), "load/test", 1, progress)
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

	err := RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress)
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
}

func TestRunSubscribe_SubscribeTimesOut(t *testing.T) {
	client := &fakeClient{subscribeToken: &fakeToken{completed: false}}
	progress := NewSubscribeProgress()

	err := RunSubscribe(context.Background(), client, discardLogger(), "load/test", 1, progress)
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
}
