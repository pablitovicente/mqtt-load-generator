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

	"github.com/pablitovicente/mqtt-load-generator/v2/internal/broker"
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
// and stats until ctx is cancelled, or until count messages have arrived (count 0 means no
// limit). Ctrl-C is the normal way to stop it: cancelling ctx is not an error. However it
// returns, including when the subscribe fails, RunSubscribe disconnects the client and marks
// progress as done.
func RunSubscribe(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	progress *SubscribeProgress,
	stats *SubscribeStats,
	count int64,
) error {
	// The display waits for progress.Done() before the command can return, so progress must be
	// finished on every return path. Deferred calls run last-in first-out: Disconnect first,
	// then finish.
	defer progress.finish()
	defer client.Disconnect(maxWaitForQueuedSends)

	countReached := make(chan struct{})

	if err := subscribeAndWait(client, topic, qos, func(_ string, payload []byte) {
		number, now := progress.recordMessage(len(payload))

		if stats.benchmark {
			recordLatency(stats, number, now, payload)
		}

		// The received counter hands out each number once, so exactly one callback sees
		// number == count and a plain close is safe.
		if count > 0 && number == count {
			close(countReached)
		}
	}); err != nil {
		return err
	}

	progress.recordSubscribed()
	logger.Info("subscribed", "topic", topic, "qos", qos)

	// The sampler stops when the run ends. Wait for it before returning, so
	// stats.receivedPerInterval is safe to read once RunSubscribe returns.
	runOver := make(chan struct{})
	samplerStopped := make(chan struct{})
	go func() {
		defer close(samplerStopped)
		stats.receivedPerInterval = sampleTotal(func() int64 { return progress.Snapshot().Received }, runOver, ackSampleInterval)
	}()

	select {
	case <-ctx.Done():
	case <-countReached:
	}

	close(runOver)
	<-samplerStopped

	return nil
}

// recordLatency works out how long a benchmark message took to arrive and counts it. The
// latency is in milliseconds, since that is what the payload timestamp holds.
func recordLatency(stats *SubscribeStats, number int64, arrivedAt time.Time, payload []byte) {
	timestamp, ok := benchmarkTimestamp(payload)
	if !ok {
		stats.withoutTimestamp.Add(1)
		return
	}

	latencyMilliseconds := arrivedAt.UnixMilli() - timestamp
	if latencyMilliseconds < 0 {
		stats.negativeLatency.Add(1)
		return
	}

	stats.latency.forClient(int(number)).record(latencyMilliseconds)
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
