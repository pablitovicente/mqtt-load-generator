package mqttload

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

// RunDump subscribes to topic at the given QoS and writes each received payload to output as
// text, one per line, until ctx is cancelled. Ctrl-C is the normal way to stop it: cancelling
// ctx is not an error. RunDump disconnects the client and returns nil once it stops.
//
// Only payloads go to output. Logs (subscribed, stopped) go to logger, so
// "mqtt-load-generator dump > file" captures nothing but payloads.
func RunDump(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	output io.Writer,
) error {
	// paho calls the message callback on its own goroutines, and more than one message can be
	// in flight at once (SetOrderMatters(false)), so two deliveries could call this at the same
	// time. Without a lock, their writes could interleave into one garbled line. Building the
	// payload and its newline into a single slice and writing it in one Write call, all under
	// the lock, keeps every line whole.
	var writeMutex sync.Mutex

	writeLine := func(_ string, payload []byte) {
		line := make([]byte, 0, len(payload)+1)
		line = append(line, payload...)
		line = append(line, '\n')

		writeMutex.Lock()
		defer writeMutex.Unlock()
		output.Write(line)
	}

	if err := subscribeAndWait(client, topic, qos, writeLine); err != nil {
		return err
	}

	logger.Info("subscribed", "topic", topic, "qos", qos)

	<-ctx.Done()

	client.Disconnect(maxWaitForQueuedSends)

	logger.Info("dump stopped")

	return nil
}
