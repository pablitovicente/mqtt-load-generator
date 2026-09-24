package mqttload

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// TestConnectClients_NeverExceedsConcurrency checks that at most connectConcurrency connects
// run at once, by having each fake connect block until the test releases it and watching how
// many are active at a time.
func TestConnectClients_NeverExceedsConcurrency(t *testing.T) {
	const clientCount = 4
	const concurrency = 2

	var mutex sync.Mutex
	active := 0
	maxActive := 0
	release := make([]chan struct{}, clientCount)
	for i := range release {
		release[i] = make(chan struct{})
	}

	connect := func(_ context.Context, clientNumber int) (Publisher, error) {
		mutex.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mutex.Unlock()

		<-release[clientNumber-1]

		mutex.Lock()
		active--
		mutex.Unlock()

		return &fakePublisher{}, nil
	}

	activeCount := func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return active
	}

	done := make(chan struct{})
	var clients []Publisher
	var err error
	go func() {
		clients, err = connectClients(context.Background(), connect, clientCount, concurrency, io.Discard)
		close(done)
	}()

	// Wait until concurrency connects are running at once, proving the limit is actually
	// exercised, then release the rest one at a time, checking the limit holds each time.
	waitFor(t, time.Second, func() bool { return activeCount() == concurrency })

	for i := 0; i < clientCount; i++ {
		close(release[i])

		if i+concurrency < clientCount {
			waitFor(t, time.Second, func() bool { return activeCount() == concurrency })
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connectClients did not return")
	}

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(clients) != clientCount {
		t.Fatalf("expected %d clients, got %d", clientCount, len(clients))
	}

	mutex.Lock()
	seenMax := maxActive
	mutex.Unlock()

	if seenMax > concurrency {
		t.Errorf("max concurrent connects = %d, want <= %d", seenMax, concurrency)
	}
	if seenMax != concurrency {
		t.Errorf("max concurrent connects = %d, want exactly %d (limit never reached)", seenMax, concurrency)
	}
}

// TestConnectClients_FailedConnectDisconnectsAlreadyConnected checks that a failed connect
// stops the run, returns the error, and disconnects every client that did connect.
func TestConnectClients_FailedConnectDisconnectsAlreadyConnected(t *testing.T) {
	const clientCount = 4

	wantErr := errors.New("dial failed")

	var mutex sync.Mutex
	var connected []*fakePublisher

	connect := func(_ context.Context, clientNumber int) (Publisher, error) {
		if clientNumber == 3 {
			return nil, wantErr
		}

		client := &fakePublisher{}
		mutex.Lock()
		connected = append(connected, client)
		mutex.Unlock()
		return client, nil
	}

	// connectConcurrency 1 makes this deterministic: clients connect in order 1, 2, 3 (fails),
	// and 4 is never attempted.
	clients, err := connectClients(context.Background(), connect, clientCount, 1, io.Discard)

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to be %v, got %v", wantErr, err)
	}
	if clients != nil {
		t.Errorf("expected no clients returned, got %v", clients)
	}

	mutex.Lock()
	defer mutex.Unlock()

	if len(connected) == 0 {
		t.Fatal("expected at least one client to have connected before the failure")
	}
	for i, client := range connected {
		if !client.wasDisconnected() {
			t.Errorf("connected client %d: expected Disconnect to be called", i)
		}
	}
}

// TestConnectClients_CtxCancelDuringConnect checks that cancelling ctx while clients are still
// connecting stops the run, returns ctx's error, and disconnects whatever did connect (nothing,
// in this test, since every connect is still in flight when ctx is cancelled).
func TestConnectClients_CtxCancelDuringConnect(t *testing.T) {
	const clientCount = 4

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{}, clientCount)

	connect := func(ctx context.Context, _ int) (Publisher, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	done := make(chan struct{})
	var clients []Publisher
	var err error
	go func() {
		clients, err = connectClients(ctx, connect, clientCount, clientCount, io.Discard)
		close(done)
	}()

	<-started // at least one connect is running
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connectClients did not return after ctx was cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if clients != nil {
		t.Errorf("expected no clients returned, got %v", clients)
	}
}
