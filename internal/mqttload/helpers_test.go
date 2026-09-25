package mqttload

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// discardLogger returns a logger that throws away everything, for tests that don't care about
// log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitFor polls condition until it returns true or the timeout passes, failing the test if it
// never does. It exists so tests don't have to guess a fixed sleep long enough for a
// background goroutine to catch up.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", timeout)
		}
		time.Sleep(time.Millisecond)
	}
}
