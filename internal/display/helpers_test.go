package display

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// discardLogger returns a logger that throws away everything, for tests that don't care about
// log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// syncBuffer is a bytes.Buffer safe for one goroutine to write to (the display loop under
// test) while the test goroutine reads it, so tests can run under -race.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(p)
}

func (b *syncBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}

// captureLogger returns a logger and the buffer its output lands in, as plain text so tests
// can check for a substring without decoding JSON.
func captureLogger() (*slog.Logger, *syncBuffer) {
	buffer := &syncBuffer{}
	return slog.New(slog.NewTextHandler(buffer, nil)), buffer
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
