package mqttload

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
	"github.com/schollz/progressbar/v3"
)

// Publisher is what the publish loop needs from a connected MQTT client. *broker.Client
// satisfies it; tests use a fake.
type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) broker.Token
	Disconnect(maxWaitForQueuedSends time.Duration)
}

// connectClientFunc connects one client, numbered 1..N. cli builds it from the parsed
// connection options, a client ID (--clientID or one generated per client) and a logger; this
// package knows nothing about broker.Dial.
type connectClientFunc func(ctx context.Context, clientNumber int) (Publisher, error)

// connectClients connects clientCount clients in parallel, at most connectConcurrency at a
// time, and shows progress on output like v1's "Connecting N MQTT clients" bar. Publishing only
// starts once every client has connected, as the README promises.
//
// If any connect fails, or ctx is cancelled first, connectClients stops starting new connects,
// waits for the ones already running to finish, disconnects every client that did connect, and
// returns the error (the connect failure, or ctx's error).
func connectClients(ctx context.Context, connect connectClientFunc, clientCount, connectConcurrency int, output io.Writer) ([]Publisher, error) {
	// A child context: cancelled either when ctx itself is cancelled, or as soon as one connect
	// fails, so the other in-flight dials stop instead of running to completion for nothing.
	connectCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	bar := progressbar.NewOptions64(int64(clientCount),
		progressbar.OptionSetDescription(fmt.Sprintf("Connecting %d MQTT clients", clientCount)),
		progressbar.OptionSetWriter(output),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(output, "\n") }),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)

	clients := make([]Publisher, clientCount)
	concurrencySlots := make(chan struct{}, connectConcurrency)

	var mutex sync.Mutex
	var firstError error
	var waitGroup sync.WaitGroup

launchLoop:
	for clientNumber := 1; clientNumber <= clientCount; clientNumber++ {
		select {
		case concurrencySlots <- struct{}{}:
		case <-connectCtx.Done():
			break launchLoop
		}

		waitGroup.Add(1)
		go func(clientNumber int) {
			defer waitGroup.Done()
			defer func() { <-concurrencySlots }()

			client, err := connect(connectCtx, clientNumber)

			mutex.Lock()
			defer mutex.Unlock()

			if err != nil {
				if firstError == nil {
					firstError = err
					cancel()
				}
				return
			}

			clients[clientNumber-1] = client
			_ = bar.Add(1)
		}(clientNumber)
	}

	waitGroup.Wait()

	if firstError != nil {
		disconnectAll(clients)
		return nil, firstError
	}

	if err := ctx.Err(); err != nil {
		disconnectAll(clients)
		return nil, err
	}

	return clients, nil
}

// disconnectAll disconnects every non-nil client in clients. Used to tear down whatever
// connected successfully when connecting as a whole fails.
func disconnectAll(clients []Publisher) {
	for _, client := range clients {
		if client != nil {
			client.Disconnect(maxWaitForQueuedSends)
		}
	}
}
