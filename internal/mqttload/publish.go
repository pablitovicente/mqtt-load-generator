package mqttload

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
	"github.com/schollz/progressbar/v3"
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
// time), then has every client publish Options.Count messages. It shows progress on output, like
// v1's "Publishing N messages" bar, and logs a summary through logger once every client stops.
//
// Ctrl-C (ctx cancelled) stops sending new messages, waits for whatever was already in flight
// (bounded by AckTimeout), disconnects, and reports what happened -- that on its own is not an
// error. RunPublish returns an error when connecting fails, or when the run ends with any failed
// or timed-out publish.
func RunPublish(ctx context.Context, connect connectClientFunc, logger *slog.Logger, options PublishOptions, output io.Writer) error {
	clients, err := connectClients(ctx, connect, options.Clients, options.ConnectConcurrency, output)
	if err != nil {
		// Ctrl-C while connecting is a normal stop, the same as Ctrl-C while publishing.
		if ctx.Err() != nil {
			logger.Info("pub stopped before every client connected")
			return nil
		}

		return fmt.Errorf("connecting clients: %w", err)
	}

	logger.Info("connected", "clients", len(clients))

	counters := make([]*publishCounters, options.Clients)
	for i := range counters {
		counters[i] = &publishCounters{}
	}

	startedAt := time.Now()

	// barDone tells the progress bar goroutine to stop; barStopped confirms it actually has,
	// including its final write to output, so nothing below (or a test reading output) races
	// with it.
	barDone := make(chan struct{})
	barStopped := make(chan struct{})
	go func() {
		defer close(barStopped)
		runPublishProgressBar(barDone, output, counters, int64(options.Clients)*int64(options.Count), progressBarUpdateInterval)
	}()

	var waitGroup sync.WaitGroup
	for i, client := range clients {
		waitGroup.Add(1)
		go func(clientNumber int, client Publisher, counters *publishCounters) {
			defer waitGroup.Done()
			runClientPublish(ctx, client, clientNumber, options, counters)
		}(i+1, client, counters[i])
	}
	waitGroup.Wait()
	close(barDone)
	<-barStopped

	elapsed := time.Since(startedAt)
	summary := sumPublishCounters(counters)

	var messagesPerSecond float64
	if elapsed > 0 {
		messagesPerSecond = float64(summary.Published) / elapsed.Seconds()
	}

	logger.Info("pub stopped",
		"published", summary.Published,
		"acked", summary.Acked,
		"failed", summary.Failed,
		"timedOut", summary.TimedOut,
		"elapsedSeconds", elapsed.Seconds(),
		"messagesPerSecond", messagesPerSecond,
	)

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

// runClientPublish runs one client's share of the load: up to Options.Count publishes, each
// waited on by its own goroutine so the client can have up to Options.InFlight publishes
// unacknowledged at once (see the in-flight window comment below). It stops sending as soon as
// ctx is cancelled, then waits for whatever was already in flight before disconnecting.
func runClientPublish(ctx context.Context, client Publisher, clientNumber int, options PublishOptions, counters *publishCounters) {
	topic := clientTopic(options.Topic, clientNumber, options.Suffix)

	generatePayload := newPayloadGenerator(options.Size, options.Benchmark)

	// Each client gets its own randomly seeded generator, so clients don't share state and
	// runs aren't identical from one process start to the next.
	random := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	clientPacer := newPacer(options.Schedule, options.IntervalMilliseconds, random)

	// inflightSlots is the in-flight window: one code path for every --inflight value,
	// including 1. Sending into it is "take a slot" (blocks once InFlight publishes are
	// outstanding); the goroutine started below for each publish releases its slot by
	// receiving from the channel once the broker has answered, or AckTimeout has passed.
	inflightSlots := make(chan struct{}, options.InFlight)

	// outstandingPublishes tracks the goroutines waiting on publish results, so the client can
	// wait for all of them before disconnecting.
	var outstandingPublishes sync.WaitGroup

sendLoop:
	for i := 0; i < options.Count; i++ {
		select {
		case inflightSlots <- struct{}{}:
		case <-ctx.Done():
			break sendLoop
		}

		token := client.Publish(topic, options.QoS, false, generatePayload())
		counters.published.Add(1)

		outstandingPublishes.Add(1)
		go func(token broker.Token) {
			defer outstandingPublishes.Done()
			defer func() { <-inflightSlots }()

			switch {
			case !token.WaitTimeout(options.AckTimeout):
				counters.timedOut.Add(1)
			case token.Error() != nil:
				counters.failed.Add(1)
			default:
				counters.acked.Add(1)
			}
		}(token)

		if !clientPacer.wait(ctx) {
			break sendLoop
		}
	}

	// Disconnect does not wait for acknowledgements itself, so wait for every outstanding
	// publish to be acked, fail or time out first.
	outstandingPublishes.Wait()

	client.Disconnect(maxWaitForQueuedSends)
}

// runPublishProgressBar shows "Publishing N messages" on output, like v1, advancing by however
// many messages were published since the last sample, until done is closed.
func runPublishProgressBar(done <-chan struct{}, output io.Writer, counters []*publishCounters, total int64, interval time.Duration) {
	bar := progressbar.NewOptions64(total,
		progressbar.OptionSetDescription(fmt.Sprintf("Publishing %d messages", total)),
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

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previous int64
	sample := func() {
		current := sumPublishCounters(counters).Published
		_ = bar.Add64(current - previous)
		previous = current
	}

	for {
		select {
		case <-done:
			sample()
			return
		case <-ticker.C:
			sample()
		}
	}
}
