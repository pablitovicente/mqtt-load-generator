package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

// TestPublishConnectsWithParsedOptions checks that pub converts its parsed connection flags
// into broker.Options and connects with them, using a generated client ID, and that the
// publish flags actually reach what gets published. The run is tiny (2 messages, no wait) so
// the test stays fast.
func TestPublishConnectsWithParsedOptions(t *testing.T) {
	connector := &fakeConnector{}

	_, err := runCommandWithConnector(t, context.Background(), connector,
		"pub", "-h", "broker", "-p", "1884", "-t", "load/custom", "-q", "2", "-c", "2", "-i", "0", "-s", "16")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
	}

	want := broker.Options{
		Host:             "broker",
		Port:             1884,
		CleanSession:     true,
		KeepAliveSeconds: 5,
	}
	if calls[0].options != want {
		t.Errorf("connect options = %+v, want %+v", calls[0].options, want)
	}
	if !strings.HasPrefix(calls[0].clientID, "mqtt-load-generator-") {
		t.Errorf("expected a generated client ID, got %q", calls[0].clientID)
	}

	publishes := connector.recordedPublishCalls()
	if len(publishes) != 2 {
		t.Fatalf("expected 2 publishes, got %d", len(publishes))
	}
	for _, call := range publishes {
		if call.topic != "load/custom" {
			t.Errorf("topic = %q, want %q", call.topic, "load/custom")
		}
		if call.qos != 2 {
			t.Errorf("qos = %d, want 2", call.qos)
		}
		if len(call.payload) != 16 {
			t.Errorf("payload size = %d, want 16", len(call.payload))
		}
	}

	if connector.disconnectCount() != 1 {
		t.Errorf("expected 1 disconnect, got %d", connector.disconnectCount())
	}
}

// TestPublishOneConnectPerClientWithDistinctIDs checks that pub connects once per client, each
// with its own generated client ID.
func TestPublishOneConnectPerClientWithDistinctIDs(t *testing.T) {
	connector := &fakeConnector{}

	_, err := runCommandWithConnector(t, context.Background(), connector,
		"pub", "-n", "3", "-c", "1", "-i", "0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := connector.recordedCalls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 connect calls, got %d", len(calls))
	}

	seenIDs := map[string]bool{}
	for _, call := range calls {
		if seenIDs[call.clientID] {
			t.Errorf("client ID %q used more than once", call.clientID)
		}
		seenIDs[call.clientID] = true
	}

	if connector.disconnectCount() != 3 {
		t.Errorf("expected 3 disconnects, got %d", connector.disconnectCount())
	}
}

// TestPublishClientIDWithSingleClient checks that --clientID is passed straight through when
// there is exactly one client, as Validate allows.
func TestPublishClientIDWithSingleClient(t *testing.T) {
	connector := &fakeConnector{}

	_, err := runCommandWithConnector(t, context.Background(), connector,
		"pub", "--clientID", "my-client", "-n", "1", "-c", "1", "-i", "0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 || calls[0].clientID != "my-client" {
		t.Fatalf("expected a single call with client ID %q, got %+v", "my-client", calls)
	}
}

// TestPublishReturnsErrorWhenConnectFails checks that a failed connection comes back as an
// error instead of panicking or exiting the process.
func TestPublishReturnsErrorWhenConnectFails(t *testing.T) {
	connector := &fakeConnector{err: errors.New("connection refused")}

	_, err := runCommandWithConnector(t, context.Background(), connector, "pub", "-c", "1", "-i", "0")
	if err == nil {
		t.Fatal("expected an error, got none")
	}
}

// TestPublishExitsNonZeroOnPublishFailure checks that pub's own exit condition (failed or
// timed-out publishes) is reachable through the CLI, not just at the mqttload level.
func TestPublishExitsNonZeroOnPublishFailure(t *testing.T) {
	connector := &fakeConnector{failPublish: errors.New("broker rejected the publish")}

	_, err := runCommandWithConnector(t, context.Background(), connector,
		"pub", "-c", "2", "-i", "0", "--ack-timeout", "50ms")
	if err == nil {
		t.Fatal("expected an error, got none")
	}
}

// TestPublishWaitsForDisplayBeforeReturning checks that by the time the command returns, the
// display goroutine has already written its final summary line: pub waits for it instead of
// racing it (see the barStopped-style channel in newPublishRunFunction).
func TestPublishWaitsForDisplayBeforeReturning(t *testing.T) {
	connector := &fakeConnector{}

	output, err := runCommandWithConnector(t, context.Background(), connector,
		"pub", "-c", "3", "-i", "0", "-n", "2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(output, "pub stopped") {
		t.Errorf("expected the summary line to already be in the output when the command returns, got: %s", output)
	}
}

// TestPublishLogsStoppingMessageOnFirstCancel checks that cancelling pub's context while a
// publish is still in flight logs the "stopping" line once, naming the real --ack-timeout
// value, instead of leaving the run looking frozen until the wait is over.
func TestPublishLogsStoppingMessageOnFirstCancel(t *testing.T) {
	connector := &fakeConnector{publishDelay: 300 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := runCommandWithConnector(t, ctx, connector,
			"pub", "-c", "100", "-i", "0", "--ack-timeout", "2s")
		done <- result{output: output, err: err}
	}()

	// Wait for the first publish to actually be in flight before cancelling, instead of
	// guessing a sleep long enough for the run to have started.
	deadline := time.Now().Add(time.Second)
	for len(connector.recordedPublishCalls()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no publish call was recorded in time")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("expected no error, got %v", got.err)
		}
		wantLine := "stopping: waiting up to 2s for in-flight publishes, press Ctrl-C again to quit"
		if !strings.Contains(got.output, wantLine) {
			t.Errorf("expected the stopping message, got: %s", got.output)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pub did not return after ctx was cancelled")
	}
}
