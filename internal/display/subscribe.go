package display

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// disableBarLogInterval is how often RunSubscribeLog re-reads progress and logs a line.
const disableBarLogInterval = 1 * time.Second

// subscribeProgressSource is what the subscribe display functions need from a subscribe run's
// progress. *mqttload.SubscribeProgress satisfies it; tests use a fake.
type subscribeProgressSource interface {
	Snapshot() mqttload.SubscribeSnapshot
	Done() <-chan struct{}
}

// RunSubscribeBar shows an indeterminate progress bar on output, advancing it by however many
// new messages arrived since the last tick, until progress reports the run is done. It logs a
// summary line once the run is done.
func RunSubscribeBar(progress subscribeProgressSource, logger *slog.Logger, output io.Writer) {
	runSubscribeBar(progress, logger, output, progressBarUpdateInterval)
}

func runSubscribeBar(progress subscribeProgressSource, logger *slog.Logger, output io.Writer, interval time.Duration) {
	_, _ = fmt.Fprintln(output, "press ctrl+c to exit")

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

	var previous int64
	for {
		select {
		case <-progress.Done():
			endBarLineIfUnfinished(bar, output)
			logSubscribeSummary(logger, progress)
			return

		case <-ticker.C:
			current := progress.Snapshot().Received
			_ = bar.Add64(current - previous)
			previous = current
		}
	}
}

// RunSubscribeLog logs a line every tick while messages are being received, and, once nothing
// has arrived for resetAfter, resets what it treats as the baseline count (with its own log
// line) so the next line reports from zero again. mqttload's own count never resets: the
// baseline, and the reset itself, are display state only. It logs a summary line once the run
// is done.
func RunSubscribeLog(progress subscribeProgressSource, logger *slog.Logger, resetAfter time.Duration) {
	runSubscribeLog(progress, logger, resetAfter, disableBarLogInterval)
}

func runSubscribeLog(progress subscribeProgressSource, logger *slog.Logger, resetAfter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var baseline int64 // received count subtracted after the last reset
	var previousCount int64

	for {
		select {
		case <-progress.Done():
			logSubscribeSummary(logger, progress)
			return

		case now := <-ticker.C:
			snapshot := progress.Snapshot()
			currentCount := snapshot.Received - baseline

			report := buildRateReport(previousCount, currentCount, snapshot.LastMessageAt, now, resetAfter)
			if report.line != "" {
				logger.Info(report.line)
			}
			if report.shouldReset {
				logger.Info("no message received for a while, resetting counter", "resetAfterSeconds", resetAfter.Seconds())
				baseline = snapshot.Received
				currentCount = 0
			}

			previousCount = currentCount
		}
	}
}

// logSubscribeSummary logs the final "sub stopped" line: the true total received over the
// whole run, regardless of any --reset-after resets shown along the way.
func logSubscribeSummary(logger *slog.Logger, progress subscribeProgressSource) {
	logger.Info("sub stopped", "totalReceived", progress.Snapshot().Received)
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
