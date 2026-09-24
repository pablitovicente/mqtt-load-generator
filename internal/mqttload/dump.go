package mqttload

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// DumpOptions holds the settings for how dump prints what it receives.
type DumpOptions struct {
	// ShowTopic prints the topic and a tab before each payload. A tab, not a space, because
	// MQTT topics may contain spaces.
	ShowTopic bool
}

// RunDump subscribes to topic at the given QoS and writes each received payload to output as
// text, one per line, until ctx is cancelled. Ctrl-C is the normal way to stop it: cancelling
// ctx is not an error. RunDump disconnects the client and returns nil once it stops.
//
// If a write to output fails (disk full, closed pipe), RunDump stops and returns that error:
// once output is broken there is no point carrying on.
//
// Only payloads go to output. Logs (subscribed, stopped) go to logger, so
// "mqtt-load-generator dump > file" captures nothing but payloads.
func RunDump(
	ctx context.Context,
	client Subscriber,
	logger *slog.Logger,
	topic string,
	qos byte,
	options DumpOptions,
	output io.Writer,
) error {
	// With --ordered=false, paho calls the callback on a new goroutine for every message, so
	// two calls can run at the same time. The lock keeps their writes from mixing. Each line is
	// built first and written in one Write call.
	var writeMutex sync.Mutex

	// The first write error is sent here and ends the run. writeFailedOnce (guarded by
	// writeMutex) makes sure only one error is sent, so the send never blocks.
	writeFailed := make(chan error, 1)
	writeFailedOnce := false

	writeLine := func(messageTopic string, payload []byte) {
		line := formatLine(messageTopic, payload, options.ShowTopic)

		writeMutex.Lock()
		defer writeMutex.Unlock()

		if writeFailedOnce {
			return
		}

		if _, err := output.Write(line); err != nil {
			writeFailedOnce = true
			writeFailed <- err
		}
	}

	if err := subscribeAndWait(client, topic, qos, writeLine); err != nil {
		return err
	}

	logger.Info("subscribed", "topic", topic, "qos", qos)

	var writeError error
	select {
	case <-ctx.Done():
	case writeError = <-writeFailed:
	}

	client.Disconnect(maxWaitForQueuedSends)

	if writeError != nil {
		return fmt.Errorf("writing payload: %w", writeError)
	}

	logger.Info("dump stopped")

	return nil
}

// formatLine builds one output line: the payload, optionally preceded by the topic and a tab,
// followed by a newline.
func formatLine(topic string, payload []byte, showTopic bool) []byte {
	line := make([]byte, 0, len(topic)+1+len(payload)+1)

	if showTopic {
		line = append(line, topic...)
		line = append(line, '\t')
	}

	line = append(line, payload...)

	return append(line, '\n')
}
