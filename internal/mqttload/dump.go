package mqttload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
)

// DumpOptions holds the settings for how dump prints what it receives.
type DumpOptions struct {
	// ShowTopic prints the topic and a tab before each payload. A tab, not a space, because
	// MQTT topics may contain spaces. Only used in the default format: JSON lines always
	// include the topic.
	ShowTopic bool

	// JSON prints each message as one JSON object per line. See formatLine.
	JSON bool
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
		line, err := formatLine(messageTopic, payload, options)
		if err != nil {
			// Only --json fails here: the payload is not valid JSON. Say so without printing
			// the payload itself, and carry on with the next message.
			logger.Warn("payload is not valid JSON, skipped", "topic", messageTopic, "size", len(payload))
			return
		}

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

// formatLine builds one output line for a received message.
//
// Payload and topic come from whoever publishes, so they are never written as they are:
// control characters such as ESC or \r would be acted on by the terminal (and by anyone who
// later cats a saved dump).
//
//   - Default: strconv.QuoteToASCII, which writes printable ASCII as itself and everything else
//     as an escape (\x1b, \r, \u202e, ...). strconv.Unquote gives back the exact bytes.
//   - JSON: one JSON object per line, {"topic":...,"payload":...}, with the payload embedded
//     as JSON. Valid JSON cannot contain raw control characters. A payload that is not valid
//     JSON returns an error and the message is skipped.
func formatLine(topic string, payload []byte, options DumpOptions) ([]byte, error) {
	if options.JSON {
		return formatJSONLine(topic, payload)
	}

	line := strconv.QuoteToASCII(string(payload)) + "\n"

	if options.ShowTopic {
		line = strconv.QuoteToASCII(topic) + "\t" + line
	}

	return []byte(line), nil
}

// formatJSONLine builds one {"topic":...,"payload":...} line. encoding/json checks that the
// payload is valid JSON and writes it on one line, keeping its key order.
func formatJSONLine(topic string, payload []byte) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)

	// Keep <, > and & as they are; the default turns them into \u003c etc., which is only
	// useful when JSON is embedded in HTML.
	encoder.SetEscapeHTML(false)

	message := struct {
		Topic   string          `json:"topic"`
		Payload json.RawMessage `json:"payload"`
	}{topic, payload}

	// Encode ends the line with a newline itself.
	if err := encoder.Encode(message); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
