package display

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

func TestRunSubscribeLog_ReportsReceivedMessagesAndSummary(t *testing.T) {
	progress := newFakeSubscribeProgress()
	logger, logs := captureLogger()

	done := make(chan struct{})
	go func() {
		runSubscribeLog(progress, logger, time.Hour, time.Millisecond)
		close(done)
	}()

	progress.set(mqttload.SubscribeSnapshot{Received: 5, LastMessageAt: time.Now()})

	waitFor(t, time.Second, func() bool {
		return strings.Contains(logs.String(), "received 5 messages so far")
	})

	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSubscribeLog did not return")
	}

	if !strings.Contains(logs.String(), "sub stopped") || !strings.Contains(logs.String(), "totalReceived=5") {
		t.Errorf("expected the summary line with the total, got: %s", logs.String())
	}
}

func TestRunSubscribeLog_ResetsAfterSilence(t *testing.T) {
	progress := newFakeSubscribeProgress()
	logger, logs := captureLogger()

	staleTime := time.Now().Add(-time.Hour)
	progress.set(mqttload.SubscribeSnapshot{Received: 5, LastMessageAt: staleTime})

	done := make(chan struct{})
	go func() {
		runSubscribeLog(progress, logger, time.Millisecond, time.Millisecond)
		close(done)
	}()

	waitFor(t, time.Second, func() bool { return strings.Contains(logs.String(), "resetting counter") })

	// After the reset, the baseline hides the previous count: a new message is reported
	// starting from 1, not 6.
	progress.set(mqttload.SubscribeSnapshot{Received: 6, LastMessageAt: time.Now()})

	waitFor(t, time.Second, func() bool { return strings.Contains(logs.String(), "received 1 messages so far") })

	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSubscribeLog did not return")
	}
}

func TestRunSubscribeLog_LogsNothingWhileCountIsZero(t *testing.T) {
	progress := newFakeSubscribeProgress()
	logger, logs := captureLogger()

	done := make(chan struct{})
	go func() {
		runSubscribeLog(progress, logger, time.Hour, time.Millisecond)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond) // let a few ticks pass with nothing received

	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSubscribeLog did not return")
	}

	if strings.Contains(logs.String(), "received") {
		t.Errorf("expected no rate line while nothing was received, got: %s", logs.String())
	}
}

func TestRunSubscribeBar_PrintsPromptAndEndsLineWhenStoppedEarly(t *testing.T) {
	progress := newFakeSubscribeProgress()
	output := &syncBuffer{}

	done := make(chan struct{})
	go func() {
		runSubscribeBar(progress, discardLogger(), output, time.Millisecond)
		close(done)
	}()

	waitFor(t, time.Second, func() bool { return strings.Contains(output.String(), "press ctrl+c to exit") })

	progress.set(mqttload.SubscribeSnapshot{Received: 3})
	time.Sleep(20 * time.Millisecond) // let a couple of ticks happen

	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSubscribeBar did not return")
	}

	if !strings.HasSuffix(output.String(), "\n") {
		t.Errorf("expected the bar's line to end with a newline, got %q", output.String())
	}
}

func TestRunSubscribeBar_LogsSummaryOnceDone(t *testing.T) {
	progress := newFakeSubscribeProgress()
	logger, logs := captureLogger()
	progress.set(mqttload.SubscribeSnapshot{Received: 42})

	done := make(chan struct{})
	go func() {
		runSubscribeBar(progress, logger, &bytes.Buffer{}, time.Millisecond)
		close(done)
	}()

	progress.finish()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSubscribeBar did not return")
	}

	if !strings.Contains(logs.String(), "sub stopped") || !strings.Contains(logs.String(), "totalReceived=42") {
		t.Errorf("expected the summary line with the total, got: %s", logs.String())
	}
}

func TestBuildRateReport(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		previousCount   int64
		currentCount    int64
		lastMessageAt   time.Time
		resetAfter      time.Duration
		wantLineEmpty   bool
		wantShouldReset bool
	}{
		{
			name:          "zero count logs nothing",
			previousCount: 0,
			currentCount:  0,
			lastMessageAt: now,
			resetAfter:    time.Second,
			wantLineEmpty: true,
		},
		{
			name:          "positive count logs a line",
			previousCount: 10,
			currentCount:  15,
			lastMessageAt: now,
			resetAfter:    time.Minute,
			wantLineEmpty: false,
		},
		{
			name:            "stale counter logs a line and resets",
			previousCount:   10,
			currentCount:    10,
			lastMessageAt:   now.Add(-2 * time.Second),
			resetAfter:      time.Second,
			wantLineEmpty:   false,
			wantShouldReset: true,
		},
		{
			name:          "exactly at the reset boundary does not reset",
			previousCount: 10,
			currentCount:  10,
			lastMessageAt: now.Add(-time.Second),
			resetAfter:    time.Second,
			wantLineEmpty: false,
		},
		{
			name:          "zero-value lastMessageAt never triggers a reset",
			previousCount: 0,
			currentCount:  3,
			lastMessageAt: time.Time{},
			resetAfter:    time.Nanosecond,
			wantLineEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildRateReport(tt.previousCount, tt.currentCount, tt.lastMessageAt, now, tt.resetAfter)

			if (report.line == "") != tt.wantLineEmpty {
				t.Errorf("line = %q, wantEmpty %v", report.line, tt.wantLineEmpty)
			}
			if report.shouldReset != tt.wantShouldReset {
				t.Errorf("shouldReset = %v, want %v", report.shouldReset, tt.wantShouldReset)
			}
		})
	}
}
