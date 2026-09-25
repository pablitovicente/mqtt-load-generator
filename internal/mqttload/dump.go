package mqttload

import (
	"context"
	"fmt"
	"log/slog"
)

// MessagePrinter is what RunDump needs to show one received message. display implements it
// with the actual formatting and the concurrency safety needed for it (paho may call the
// subscribe callback from many goroutines at once); RunDump itself only calls Print.
type MessagePrinter interface {
	// Print handles one received message. It returns an error only when writing the message
	// out failed (disk full, closed pipe); a message it chooses to skip (invalid JSON, say)
	// is not an error.
	Print(topic string, payload []byte) error
}

// RunDump subscribes to topic at the given QoS and hands each received payload to printer,
// until ctx is cancelled. Ctrl-C is the normal way to stop it: cancelling ctx is not an error.
// RunDump disconnects the client and returns nil once it stops.
//
// If printer.Print returns an error, RunDump stops and returns it: once output is broken there
// is no point carrying on.
func RunDump(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	printer MessagePrinter,
) error {
	// printFailed only ever needs to carry the first error: display's own MessagePrinter stops
	// writing after its first failure, so later Print calls return nil. The select-with-default
	// send is a defensive backstop in case some other MessagePrinter doesn't make that promise;
	// either way this send never blocks.
	printFailed := make(chan error, 1)

	onMessage := func(messageTopic string, payload []byte) {
		if err := printer.Print(messageTopic, payload); err != nil {
			select {
			case printFailed <- err:
			default:
			}
		}
	}

	if err := subscribeAndWait(client, topic, qos, onMessage); err != nil {
		client.Disconnect(maxWaitForQueuedSends)
		return err
	}

	logger.Info("subscribed", "topic", topic, "qos", qos)

	var printError error
	select {
	case <-ctx.Done():
	case printError = <-printFailed:
	}

	client.Disconnect(maxWaitForQueuedSends)

	if printError != nil {
		return fmt.Errorf("writing payload: %w", printError)
	}

	logger.Info("dump stopped")

	return nil
}
