package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// connectCall records one call to fakeConnector.connect.
type connectCall struct {
	options  broker.Options
	clientID string
}

// fakeConnector stands in for connectToBroker. It records what it was called with and hands
// back a subscriber whose subscribe always succeeds, or fails with err if that is set.
type fakeConnector struct {
	mutex sync.Mutex
	calls []connectCall
	err   error
}

func (connector *fakeConnector) connect(_ context.Context, options broker.Options, clientID string, _ *slog.Logger) (mqttload.Subscriber, error) {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	connector.calls = append(connector.calls, connectCall{options: options, clientID: clientID})

	if connector.err != nil {
		return nil, connector.err
	}
	return fakeSubscriber{}, nil
}

func (connector *fakeConnector) recordedCalls() []connectCall {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	return append([]connectCall(nil), connector.calls...)
}

// fakeSubscriber accepts every subscribe and ignores disconnect. The cli tests only check what
// sub connected with; mqttload's own tests cover the sub loop.
type fakeSubscriber struct{}

func (fakeSubscriber) Subscribe(_ string, _ byte, _ func(topic string, payload []byte)) broker.Token {
	return succeededToken{}
}

func (fakeSubscriber) Disconnect(_ time.Duration) {}

// succeededToken is a broker.Token that has already completed without error.
type succeededToken struct{}

func (succeededToken) WaitTimeout(_ time.Duration) bool { return true }
func (succeededToken) Error() error                     { return nil }

// printedConfig matches the JSON that printConfig writes, so tests can read it back as typed
// values instead of a map[string]any with type assertions.
type printedConfig struct {
	Connection Connection `json:"connection"`
	Publish    *Publish   `json:"publish"`
	Subscribe  *Subscribe `json:"subscribe"`
}

// runCommand builds a fresh root command, runs it with the given args, and returns everything
// written to stdout/stderr together with any error from Execute. A fresh command is needed
// for every call because flag values live on the Connection/Publish/Subscribe values captured
// by NewRootCommand's closures.
//
// It runs with a fake connector that connects successfully, and a context that is never
// cancelled. That's fine for every command except sub, which runs until its context is
// cancelled: tests that exercise sub use runCommandWithConnector instead, with a context they
// control.
func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	return runCommandWithConnector(t, context.Background(), &fakeConnector{}, args...)
}

// runCommandWithConnector is like runCommand, but lets the caller supply the connector and the
// context, for tests that need to inspect what sub connected with or control when sub's run
// loop stops.
func runCommandWithConnector(t *testing.T, ctx context.Context, connector *fakeConnector, args ...string) (string, error) {
	t.Helper()

	rootCommand := newRootCommand(connector.connect)

	var output bytes.Buffer
	rootCommand.SetOut(&output)
	rootCommand.SetErr(&output)
	rootCommand.SetArgs(args)

	err := rootCommand.ExecuteContext(ctx)
	return output.String(), err
}

// decodeConfig parses the JSON a command printed into a printedConfig. It fails the test if
// the output is not valid JSON.
func decodeConfig(t *testing.T, output string) printedConfig {
	t.Helper()

	var config printedConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		t.Fatalf("could not decode config JSON: %v\noutput: %s", err, output)
	}

	return config
}
