package mqttload

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/pablitovicente/mqtt-load-generator/v2/internal/broker"
)

// Publisher is what the publish loop needs from a connected MQTT client. *broker.Client
// satisfies it; tests use a fake.
type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) broker.Token
	Disconnect(maxWaitForQueuedSends time.Duration)
	ClientID() string
}

// connectClientFunc connects one client, numbered 1..N. cli builds it from the parsed
// connection options, a client ID (--clientID or one generated per client) and a logger; this
// package knows nothing about broker.Dial.
type connectClientFunc func(ctx context.Context, clientNumber int) (Publisher, error)

// connectClients connects clientCount clients in parallel, at most connectConcurrency at a
// time, and records each one as it connects on progress, for display to show. It also stores
// each client's connect time and client ID in stats. Publishing only starts once every client
// has connected, as the README promises.
//
// If any connect fails, or ctx is cancelled first, connectClients stops starting new connects,
// waits for the ones already running to finish, disconnects every client that did connect, and
// returns the error (the connect failure, or ctx's error).
func connectClients(ctx context.Context, connect connectClientFunc, clientCount, connectConcurrency int, progress *PublishProgress, stats *PublishStats) ([]Publisher, error) {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(connectConcurrency)

	clients := make([]Publisher, clientCount)

	for clientNumber := 1; clientNumber <= clientCount; clientNumber++ {
		group.Go(func() error {
			// Once one connect has failed (or ctx was cancelled), groupCtx is already
			// cancelled by the time a queued connect gets its turn: skip it instead of
			// dialing for nothing.
			if err := groupCtx.Err(); err != nil {
				return err
			}

			connectStartedAt := time.Now()

			client, err := connect(groupCtx, clientNumber)
			if err != nil {
				return err
			}

			connectedStats := stats.clients[clientNumber-1]
			connectedStats.connectDuration = time.Since(connectStartedAt)
			connectedStats.clientID = client.ClientID()

			clients[clientNumber-1] = client
			progress.clientConnected()
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		disconnectAll(clients)
		return nil, err
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
