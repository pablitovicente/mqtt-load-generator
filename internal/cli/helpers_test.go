package cli

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
)

// connectCall records one call to fakeConnector.connect.
type connectCall struct {
	options  broker.Options
	clientID string
}

// fakePublishCall records one publish made through a fakeConnectedClient, across every client a
// fakeConnector handed out, so pub tests can check what was actually sent.
type fakePublishCall struct {
	clientID string
	topic    string
	qos      byte
	payload  []byte
}

// fakeConnector stands in for connectToBroker. It records what it was called with and hands back
// a connectedClient that accepts every subscribe and publish, recording publishes on the connector
// itself, or fails with err if that is set.
type fakeConnector struct {
	mutex sync.Mutex

	calls        []connectCall
	err          error
	publishCalls []fakePublishCall
	disconnects  int

	// failPublish, if set, is returned as the error on every publish's token, so tests can
	// check the failed counter and pub's exit code without a real broker.
	failPublish error

	// subscribeError, if set, makes every Subscribe on the clients it hands out fail with it.
	subscribeError error
}

func (connector *fakeConnector) connect(_ context.Context, options broker.Options, clientID string, _ *slog.Logger) (connectedClient, error) {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	connector.calls = append(connector.calls, connectCall{options: options, clientID: clientID})

	if connector.err != nil {
		return nil, connector.err
	}
	return &fakeConnectedClient{connector: connector, clientID: clientID}, nil
}

func (connector *fakeConnector) recordedCalls() []connectCall {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	return append([]connectCall(nil), connector.calls...)
}

// recordedPublishCalls returns a copy of every publish made by any client this connector handed
// out.
func (connector *fakeConnector) recordedPublishCalls() []fakePublishCall {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	return append([]fakePublishCall(nil), connector.publishCalls...)
}

// disconnectCount returns how many times Disconnect was called across every client this
// connector handed out.
func (connector *fakeConnector) disconnectCount() int {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	return connector.disconnects
}

// fakeConnectedClient is what fakeConnector hands back for one connected client: it satisfies
// connectedClient (subscribe, publish and disconnect), recording what happened on the shared
// connector so a test can inspect every client's activity in one place.
type fakeConnectedClient struct {
	connector *fakeConnector
	clientID  string
}

func (client *fakeConnectedClient) Subscribe(_ string, _ byte, _ func(topic string, payload []byte)) broker.Token {
	if client.connector.subscribeError != nil {
		return failedToken{err: client.connector.subscribeError}
	}
	return succeededToken{}
}

func (client *fakeConnectedClient) Publish(topic string, qos byte, _ bool, payload []byte) broker.Token {
	client.connector.mutex.Lock()
	client.connector.publishCalls = append(client.connector.publishCalls, fakePublishCall{
		clientID: client.clientID,
		topic:    topic,
		qos:      qos,
		payload:  append([]byte(nil), payload...),
	})
	failPublish := client.connector.failPublish
	client.connector.mutex.Unlock()

	if failPublish != nil {
		return failedToken{err: failPublish}
	}
	return succeededToken{}
}

func (client *fakeConnectedClient) Disconnect(_ time.Duration) {
	client.connector.mutex.Lock()
	client.connector.disconnects++
	client.connector.mutex.Unlock()
}

// succeededToken is a broker.Token that has already completed without error.
type succeededToken struct{}

func (succeededToken) WaitTimeout(_ time.Duration) bool { return true }
func (succeededToken) Error() error                     { return nil }

// failedToken is a broker.Token that has already completed with err.
type failedToken struct{ err error }

func (failedToken) WaitTimeout(_ time.Duration) bool { return true }
func (token failedToken) Error() error               { return token.err }

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
// context, for tests that need to inspect what a command connected with or control when a
// run loop stops.
func runCommandWithConnector(t *testing.T, ctx context.Context, connector *fakeConnector, args ...string) (string, error) {
	t.Helper()

	rootCommand := newRootCommand(connector.connect)

	// A plain bytes.Buffer isn't safe here: pub and sub both run a display goroutine that
	// writes to this same output while the command's own goroutine logs to it too.
	output := &syncBuffer{}
	rootCommand.SetOut(output)
	rootCommand.SetErr(output)
	rootCommand.SetArgs(args)

	err := rootCommand.ExecuteContext(ctx)
	return output.String(), err
}

// syncBuffer is a bytes.Buffer safe for concurrent use: pub and sub write to it from both the
// command's own goroutine (logging) and a display goroutine (bars and log lines) at once.
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
