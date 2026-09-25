// Package mqttload holds the run loops used by the CLI commands: sub counts received
// messages, dump prints them, pub sends them. None of it prints anything itself: it exposes
// progress values (or, for dump, a small printer interface) that internal/display reads or
// calls to show the user what is happening.
package mqttload

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

// Subscriber is what the sub loop needs from a connected MQTT client. *broker.Client
// satisfies it; tests use a fake.
type Subscriber interface {
	Subscribe(topic string, qos byte, callback func(topic string, payload []byte)) broker.Token
	Disconnect(maxWaitForQueuedSends time.Duration)
}

// subscribeTimeout is how long RunSubscribe and RunDump wait for the broker to acknowledge the
// subscribe request before giving up.
const subscribeTimeout = 10 * time.Second

// maxWaitForQueuedSends is how long Disconnect is given to flush anything still queued, shared
// by RunSubscribe and RunDump.
const maxWaitForQueuedSends = 250 * time.Millisecond

// RunSubscribe subscribes to topic at the given QoS and records what it receives in progress
// until ctx is cancelled. Ctrl-C is the normal way to stop it: cancelling ctx is not an error.
// However it returns, including when the subscribe fails, RunSubscribe disconnects the client
// and marks progress as done.
func RunSubscribe(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	progress *SubscribeProgress,
) error {
	// The display waits for progress.Done() before the command can return, so progress must be
	// finished on every return path. Deferred calls run last-in first-out: Disconnect first,
	// then finish.
	defer progress.finish()
	defer client.Disconnect(maxWaitForQueuedSends)

	if err := subscribeAndWait(client, topic, qos, func(_ string, _ []byte) {
		progress.recordMessage()
	}); err != nil {
		return err
	}

	progress.recordSubscribed()
	logger.Info("subscribed", "topic", topic, "qos", qos)

	<-ctx.Done()

	return nil
}

// subscribeAndWait subscribes to topic at qos with callback, and waits for the broker to
// acknowledge the subscribe request. RunSubscribe and RunDump both need exactly this, so it's
// pulled out here instead of copied.
func subscribeAndWait(client Subscriber, topic string, qos byte, callback func(topic string, payload []byte)) error {
	token := client.Subscribe(topic, qos, callback)

	if !token.WaitTimeout(subscribeTimeout) {
		return fmt.Errorf("subscribing to %q: timed out waiting for the broker to acknowledge", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribing to %q: %w", topic, err)
	}

	return nil
}
