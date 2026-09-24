// Package mqttload holds the run loops used by the CLI commands: sub counts received
// messages, dump prints them, pub sends them. Iteration 1 only implements sub.
package mqttload

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
	"github.com/schollz/progressbar/v3"
)

// Subscriber is what the sub loop needs from a connected MQTT client. *broker.Client
// satisfies it; tests use a fake.
type Subscriber interface {
	Subscribe(topic string, qos byte, callback func(topic string, payload []byte)) broker.Token
	Disconnect(quiesceMilliseconds uint)
}

// SubOptions holds the settings for how the sub loop reports what it counted. It is a small
// copy of cli.Subscribe's fields, kept separate so this package doesn't need to import cli.
type SubOptions struct {
	DisableBar bool
	ResetAfter time.Duration
}

// subscribeTimeout is how long RunSub waits for the broker to acknowledge the subscribe
// request before giving up.
const subscribeTimeout = 10 * time.Second

// Real-world reporting intervals, used by RunSub. Tests call runSub directly with much
// shorter intervals so they don't have to wait on real 100ms/1s ticks.
const (
	progressBarUpdateInterval = 100 * time.Millisecond
	disableBarLogInterval     = 1 * time.Second
)

// messageCounter is the state shared between the subscribe callback -- which paho calls on its own
// goroutines, one per incoming message -- and the reporting loop that reads it. Both fields
// are plain atomics; there is no lock and no other shared mutable state, so the callback never
// blocks on the reporting loop or vice versa.
type messageCounter struct {
	received      atomic.Int64
	lastMessageAt atomic.Int64 // UnixNano; 0 means "no message received yet"
}

func (counter *messageCounter) recordMessage() {
	counter.received.Add(1)
	counter.lastMessageAt.Store(time.Now().UnixNano())
}

func (counter *messageCounter) snapshot() (count int64, lastMessageAt time.Time) {
	count = counter.received.Load()

	nano := counter.lastMessageAt.Load()
	if nano == 0 {
		return count, time.Time{}
	}
	return count, time.Unix(0, nano)
}

func (counter *messageCounter) reset() {
	counter.received.Store(0)
}

// RunSub subscribes to topic at the given QoS and reports what it receives until ctx is
// cancelled. Ctrl-C is the normal way to stop it: cancelling ctx is not an error. RunSub
// disconnects the client and returns nil once it stops.
func RunSub(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	options SubOptions,
	output io.Writer,
) error {
	return runSub(ctx, client, logger, topic, qos, options, output, progressBarUpdateInterval, disableBarLogInterval)
}

// runSub does the real work. barInterval and logInterval are parameters, rather than the
// constants above, purely so tests can drive the reporting loops without waiting on real
// 100ms/1s ticks.
func runSub(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	options SubOptions,
	output io.Writer,
	barInterval time.Duration,
	logInterval time.Duration,
) error {
	var counter messageCounter

	token := client.Subscribe(topic, qos, func(_ string, _ []byte) {
		counter.recordMessage()
	})

	if !token.WaitTimeout(subscribeTimeout) {
		return fmt.Errorf("subscribing to %q: timed out waiting for the broker to acknowledge", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribing to %q: %w", topic, err)
	}

	logger.Info("subscribed", "topic", topic, "qos", qos)

	if options.DisableBar {
		runDisableBarReporting(ctx, logger, &counter, options.ResetAfter, logInterval)
	} else {
		fmt.Fprintln(output, "press ctrl+c to exit")
		runProgressBar(ctx, output, &counter, barInterval)
	}

	client.Disconnect(250)

	logger.Info("sub stopped", "totalReceived", counter.received.Load())

	return nil
}

// runProgressBar shows an indeterminate progress bar on output (schollz/progressbar, like v1's
// progressbar.Default(-1)), advancing it by however many new messages arrived since the last
// tick, until ctx is cancelled.
func runProgressBar(ctx context.Context, output io.Writer, counter *messageCounter, interval time.Duration) {
	bar := progressbar.NewOptions64(-1,
		progressbar.OptionSetWriter(output),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previousCount int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentCount, _ := counter.snapshot()
			_ = bar.Add64(currentCount - previousCount)
			previousCount = currentCount
		}
	}
}

// runDisableBarReporting logs a line every tick while messages are being received, and resets
// the counter (with its own log line) once nothing has arrived for resetAfter. This matches
// v1's checker, minus its data race: v1 read and wrote msgCount/tickTime from two goroutines
// with no synchronization at all.
func runDisableBarReporting(ctx context.Context, logger *slog.Logger, counter *messageCounter, resetAfter time.Duration, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previousCount int64

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			currentCount, lastMessageAt := counter.snapshot()

			report := buildRateReport(previousCount, currentCount, lastMessageAt, now, resetAfter)
			if report.line != "" {
				logger.Info(report.line)
			}
			if report.shouldReset {
				logger.Info("no message received for a while, resetting counter", "resetAfterSeconds", resetAfter.Seconds())
				counter.reset()
				currentCount = 0
			}

			previousCount = currentCount
		}
	}
}

// rateReport is what one reporting tick decided to do, as a plain value so the decision logic
// in buildRateReport can be tested without a real ticker or real sleeps.
type rateReport struct {
	line        string
	shouldReset bool
}

// buildRateReport decides what a --disable-bar tick should log, from two counter samples one
// tick apart. It logs nothing while the count is 0. When the count has gone stale (no message
// for more than resetAfter), it reports both the "received so far" line and the reset, in the
// same tick, matching v1's checker.
func buildRateReport(previousCount, currentCount int64, lastMessageAt, now time.Time, resetAfter time.Duration) rateReport {
	if currentCount == 0 {
		return rateReport{}
	}

	rate := currentCount - previousCount
	line := fmt.Sprintf("received %d messages so far, %d msg/sec", currentCount, rate)

	if !lastMessageAt.IsZero() && now.Sub(lastMessageAt) > resetAfter {
		return rateReport{line: line, shouldReset: true}
	}

	return rateReport{line: line}
}
