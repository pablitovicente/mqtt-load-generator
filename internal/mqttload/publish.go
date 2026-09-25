package mqttload

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

// PublishOptions holds the settings for the publish loop: a plain copy of the relevant fields
// from cli.Connection and cli.Publish, kept separate so this package doesn't need to import cli.
type PublishOptions struct {
	Topic string
	QoS   byte

	Count                int
	Size                 int
	IntervalMilliseconds int
	Schedule             string
	Clients              int
	Suffix               bool
	Benchmark            bool

	InFlight           int
	AckTimeout         time.Duration
	ConnectConcurrency int
}

// RunPublish connects Options.Clients clients (through connect, at most ConnectConcurrency at a
// time), then has every client publish Options.Count messages, reporting what happens on
// progress as it goes. RunPublish returns an error when connecting fails, or when the run ends
// with any failed or timed-out publish.
//
// Ctrl-C (ctx cancelled) stops sending new messages, waits for whatever was already in flight
// (bounded by AckTimeout), disconnects, and reports what happened -- that on its own is not an
// error.
func RunPublish(ctx context.Context, connect connectClientFunc, logger *slog.Logger, options PublishOptions, progress *PublishProgress) error {
	clients, err := connectClients(ctx, connect, options.Clients, options.ConnectConcurrency, progress)
	if err != nil {
		progress.finishConnecting(false)
		progress.finish()

		// Ctrl-C while connecting is a normal stop, the same as Ctrl-C while publishing.
		if ctx.Err() != nil {
			logger.Info("pub stopped before every client connected")
			return nil
		}

		return fmt.Errorf("connecting clients: %w", err)
	}

	logger.Info("connected", "clients", len(clients))
	progress.finishConnecting(true)

	var waitGroup sync.WaitGroup
	for i, client := range clients {
		waitGroup.Add(1)
		go func(clientNumber int, client Publisher, counters *publishCounters) {
			defer waitGroup.Done()
			runClientPublish(ctx, client, clientNumber, options, counters)
		}(i+1, client, progress.counterFor(i))
	}
	waitGroup.Wait()

	progress.finish()

	summary := progress.Snapshot()
	if summary.Failed+summary.TimedOut > 0 {
		return fmt.Errorf("pub: %d published, %d acked, %d failed, %d timed out",
			summary.Published, summary.Acked, summary.Failed, summary.TimedOut)
	}

	return nil
}

// clientTopic returns the topic one client publishes to: --topic as-is, or with the client
// number (1..N) appended as a sub-topic when --suffix is set.
func clientTopic(topic string, clientNumber int, suffix bool) string {
	if !suffix {
		return topic
	}
	return fmt.Sprintf("%s/%d", topic, clientNumber)
}

// runClientPublish runs one client's share of the load: up to Options.Count publishes, with up
// to Options.InFlight of them unacknowledged at once. It stops sending as soon as ctx is
// cancelled, then waits for whatever was already in flight before disconnecting.
func runClientPublish(ctx context.Context, client Publisher, clientNumber int, options PublishOptions, counters *publishCounters) {
	// Each client gets its own randomly seeded generator, so clients don't share state and
	// runs aren't identical from one process start to the next.
	random := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

	publisher := &publishingClient{
		client:          client,
		topic:           clientTopic(options.Topic, clientNumber, options.Suffix),
		generatePayload: newPayloadGenerator(options.Size, options.Benchmark),
		pacer:           newPacer(options.Schedule, options.IntervalMilliseconds, random),
		options:         options,
		counters:        counters,
		inflight:        semaphore.NewWeighted(int64(options.InFlight)),
	}

	publisher.sendMessages(ctx)

	// Disconnect does not wait for acknowledgements itself, so wait for every outstanding
	// publish to be acked, fail or time out first.
	publisher.outstandingPublishes.Wait()

	client.Disconnect(maxWaitForQueuedSends)
}

// publishingClient holds what one client needs while it publishes.
type publishingClient struct {
	client          Publisher
	topic           string
	generatePayload payloadGenerator
	pacer           *pacer
	options         PublishOptions
	counters        *publishCounters

	// inflight is the in-flight window: one code path for every --inflight value, including
	// 1. Acquire is "take a slot" (blocks once InFlight publishes are outstanding, and stops
	// blocking with an error once ctx is cancelled); waitForResult releases the slot once the
	// broker has answered, or AckTimeout has passed.
	inflight *semaphore.Weighted

	// outstandingPublishes counts the waitForResult goroutines still running, so the client
	// can wait for all of them before disconnecting.
	outstandingPublishes sync.WaitGroup
}

// sendMessages publishes up to Options.Count messages, pacing them by --schedule. It returns
// early when ctx is cancelled.
func (publisher *publishingClient) sendMessages(ctx context.Context) {
	for i := 0; i < publisher.options.Count; i++ {
		if err := publisher.inflight.Acquire(ctx, 1); err != nil {
			return
		}

		token := publisher.client.Publish(publisher.topic, publisher.options.QoS, false, publisher.generatePayload())
		publisher.counters.published.Add(1)

		publisher.outstandingPublishes.Add(1)
		go publisher.waitForResult(token)

		if !publisher.pacer.wait(ctx) {
			return
		}
	}
}

// waitForResult waits for one publish to be acked, fail or time out, counts the result, and
// frees its in-flight slot.
func (publisher *publishingClient) waitForResult(token broker.Token) {
	defer publisher.outstandingPublishes.Done()
	defer publisher.inflight.Release(1)

	switch {
	case !token.WaitTimeout(publisher.options.AckTimeout):
		publisher.counters.timedOut.Add(1)
	case token.Error() != nil:
		publisher.counters.failed.Add(1)
	default:
		publisher.counters.acked.Add(1)
	}
}
