// Package display renders everything the user sees: the connect and publish progress bars,
// sub's progress bar and its --disable-bar log lines, the pub and sub summary log lines, and
// dump's per-message output. It reads progress values that mqttload exposes (or, for dump,
// implements the small printer interface mqttload declares); mqttload never imports this
// package or calls into it.
package display

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// progressBarUpdateInterval is how often the bars in this package re-read progress and redraw.
const progressBarUpdateInterval = 100 * time.Millisecond

// publishProgressSource is what the publish display functions need from a publish run's
// progress. *mqttload.PublishProgress satisfies it; tests use a fake.
type publishProgressSource interface {
	Snapshot() mqttload.PublishSnapshot
	ConnectFinished() <-chan struct{}
	Done() <-chan struct{}
}

// RunPublishProgress shows a "Connecting N clients" bar while progress is connecting, then a
// "Publishing N messages" bar once sending starts, and logs a summary line once the run is
// done. totalMessages is the number of messages the whole run is expected to send (clients
// times count per client), used as the publishing bar's maximum.
//
// It returns once the run is done and everything has been written to output and logger: the
// caller should wait for it before the command returns, so no output is lost or arrives after
// the process has already exited.
func RunPublishProgress(progress publishProgressSource, totalMessages int64, logger *slog.Logger, output io.Writer) {
	runPublishProgress(progress, totalMessages, logger, output, progressBarUpdateInterval)
}

func runPublishProgress(progress publishProgressSource, totalMessages int64, logger *slog.Logger, output io.Writer, interval time.Duration) {
	startedPublishing := runConnectBar(progress, output, interval)
	if !startedPublishing {
		// Connecting failed or was cancelled: mqttload has already logged (or, for a plain
		// connect failure, will surface its own error) -- nothing more to show here.
		return
	}

	runPublishBar(progress, totalMessages, output, interval)
	logPublishSummary(logger, progress)
}

// runConnectBar shows the "Connecting N clients" bar until progress reports connecting is
// finished. It returns whether publishing actually started.
func runConnectBar(progress publishProgressSource, output io.Writer, interval time.Duration) bool {
	total := progress.Snapshot().ClientsTotal

	bar := progressbar.NewOptions64(int64(total),
		progressbar.OptionSetDescription(fmt.Sprintf("Connecting %d MQTT clients", total)),
		progressbar.OptionSetWriter(output),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() { _, _ = fmt.Fprint(output, "\n") }),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previous int64
	for {
		select {
		case <-progress.ConnectFinished():
			snapshot := progress.Snapshot()
			_ = bar.Add64(int64(snapshot.ClientsConnected) - previous)
			endBarLineIfUnfinished(bar, output)
			return !snapshot.ConnectFailed

		case <-ticker.C:
			current := int64(progress.Snapshot().ClientsConnected)
			_ = bar.Add64(current - previous)
			previous = current
		}
	}
}

// runPublishBar shows the "Publishing N messages" bar until progress reports the run is done.
func runPublishBar(progress publishProgressSource, totalMessages int64, output io.Writer, interval time.Duration) {
	bar := progressbar.NewOptions64(totalMessages,
		progressbar.OptionSetDescription(fmt.Sprintf("Publishing %d messages", totalMessages)),
		progressbar.OptionSetWriter(output),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() { _, _ = fmt.Fprint(output, "\n") }),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previous int64
	sample := func() {
		current := progress.Snapshot().Published
		_ = bar.Add64(current - previous)
		previous = current
	}

	for {
		select {
		case <-progress.Done():
			sample()
			endBarLineIfUnfinished(bar, output)
			return

		case <-ticker.C:
			sample()
		}
	}
}

// logPublishSummary logs the final "pub stopped" line: counts, elapsed time and throughput for
// the publishing phase.
func logPublishSummary(logger *slog.Logger, progress publishProgressSource) {
	snapshot := progress.Snapshot()

	elapsed := snapshot.FinishedAt.Sub(snapshot.PublishStartedAt)

	var messagesPerSecond float64
	if elapsed > 0 {
		messagesPerSecond = float64(snapshot.Published) / elapsed.Seconds()
	}

	logger.Info("pub stopped",
		"published", snapshot.Published,
		"acked", snapshot.Acked,
		"failed", snapshot.Failed,
		"timedOut", snapshot.TimedOut,
		"elapsedSeconds", elapsed.Seconds(),
		"messagesPerSecond", messagesPerSecond,
	)
}

// endBarLineIfUnfinished writes a trailing newline to output if bar stopped before reaching its
// maximum, so whatever prints next doesn't glue onto the bar's line. A bar that did reach its
// maximum already wrote its own newline through OptionOnCompletion.
func endBarLineIfUnfinished(bar *progressbar.ProgressBar, output io.Writer) {
	if !bar.IsFinished() {
		_, _ = fmt.Fprintln(output)
	}
}
