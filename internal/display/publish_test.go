package display

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// TestRunPublishProgress_LogsSummaryOnceDone checks that once a run finishes publishing,
// RunPublishProgress logs the "pub stopped" summary with the final counts.
func TestRunPublishProgress_LogsSummaryOnceDone(t *testing.T) {
	progress := newFakePublishProgress()
	logger, logs := captureLogger()

	startedAt := time.Now().Add(-time.Second)
	finishedAt := time.Now()

	progress.set(mqttload.PublishSnapshot{
		ClientsConnected: 2, ClientsTotal: 2,
		PublishStartedAt: startedAt,
	})

	done := make(chan struct{})
	go func() {
		runPublishProgress(progress, 10, logger, &bytes.Buffer{}, time.Millisecond)
		close(done)
	}()

	progress.finishConnecting()

	progress.set(mqttload.PublishSnapshot{
		ClientsConnected: 2, ClientsTotal: 2,
		Published: 10, Acked: 9, Failed: 1,
		PublishStartedAt: startedAt,
		FinishedAt:       finishedAt,
	})
	progress.finish()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runPublishProgress did not return")
	}

	if !strings.Contains(logs.String(), "pub stopped") {
		t.Errorf("expected a summary line, got: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "failed=1") {
		t.Errorf("expected the failed count in the summary, got: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "published=10") {
		t.Errorf("expected the published count in the summary, got: %s", logs.String())
	}
}

// TestRunPublishProgress_NoSummaryWhenConnectFails checks that when connecting never gets to
// publishing, RunPublishProgress does not log a "pub stopped" summary (mqttload itself already
// says what happened in that case).
func TestRunPublishProgress_NoSummaryWhenConnectFails(t *testing.T) {
	progress := newFakePublishProgress()
	logger, logs := captureLogger()

	progress.set(mqttload.PublishSnapshot{ClientsConnected: 1, ClientsTotal: 3, ConnectFailed: true})

	done := make(chan struct{})
	go func() {
		runPublishProgress(progress, 30, logger, &bytes.Buffer{}, time.Millisecond)
		close(done)
	}()

	progress.finishConnecting()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPublishProgress did not return")
	}

	if strings.Contains(logs.String(), "pub stopped") {
		t.Errorf("expected no summary line, got: %s", logs.String())
	}
}

// TestRunPublishProgress_EndsConnectBarLineWhenStoppedEarly checks the fix for the bar line not
// being ended when a run stops before the connect bar reaches its maximum.
func TestRunPublishProgress_EndsConnectBarLineWhenStoppedEarly(t *testing.T) {
	progress := newFakePublishProgress()
	output := &syncBuffer{}

	progress.set(mqttload.PublishSnapshot{ClientsConnected: 1, ClientsTotal: 5, ConnectFailed: true}) // never reaches 5

	done := make(chan struct{})
	go func() {
		runPublishProgress(progress, 50, discardLogger(), output, time.Millisecond)
		close(done)
	}()

	waitFor(t, time.Second, func() bool { return output.String() != "" })
	progress.finishConnecting()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPublishProgress did not return")
	}

	if !strings.HasSuffix(output.String(), "\n") {
		t.Errorf("expected the connect bar's line to end with a newline, got %q", output.String())
	}
}

// TestRunPublishProgress_EndsPublishBarLineWhenStoppedEarly checks the same fix for the
// publishing bar.
func TestRunPublishProgress_EndsPublishBarLineWhenStoppedEarly(t *testing.T) {
	progress := newFakePublishProgress()
	output := &syncBuffer{}

	startedAt := time.Now()
	progress.set(mqttload.PublishSnapshot{ClientsConnected: 1, ClientsTotal: 1, PublishStartedAt: startedAt})

	done := make(chan struct{})
	go func() {
		runPublishProgress(progress, 1000, discardLogger(), output, time.Millisecond)
		close(done)
	}()

	progress.finishConnecting()

	waitFor(t, time.Second, func() bool { return output.String() != "" })

	progress.set(mqttload.PublishSnapshot{
		ClientsConnected: 1, ClientsTotal: 1,
		Published:        5,
		PublishStartedAt: startedAt,
		FinishedAt:       time.Now(),
	})
	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPublishProgress did not return")
	}

	if !strings.HasSuffix(output.String(), "\n") {
		t.Errorf("expected the publish bar's line to end with a newline, got %q", output.String())
	}
}
