package display

import (
	"bytes"
	"encoding/json"
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

	// JSON prints each message as one JSON object per line. See formatDumpLine.
	JSON bool
}

// messagePrinter implements mqttload.MessagePrinter: it formats and writes one message per
// call. Safe for concurrent calls: with --ordered off, paho calls the subscribe callback on a
// new goroutine for every message, so two calls can land here at the same time.
type messagePrinter struct {
	mutex sync.Mutex

	output  io.Writer
	logger  *slog.Logger
	options DumpOptions

	// failed is set once a write fails, so later calls stop trying (and stop returning an
	// error: only the first failure needs reporting).
	failed bool
}

// NewMessagePrinter builds the printer dump uses: it writes to output, following options, and
// warns through logger about payloads it has to skip.
func NewMessagePrinter(output io.Writer, logger *slog.Logger, options DumpOptions) *messagePrinter {
	return &messagePrinter{output: output, logger: logger, options: options}
}

// Print builds one line for the message and writes it to output. It returns an error only when
// the write itself fails; a payload it skips (invalid JSON with --json) is logged as a warning
// instead, without the payload itself, and is not an error.
func (printer *messagePrinter) Print(topic string, payload []byte) error {
	line, err := formatDumpLine(topic, payload, printer.options)
	if err != nil {
		// Only --json fails here: the payload is not valid JSON. Say so without printing the
		// payload itself, and carry on with the next message.
		printer.logger.Warn("payload is not valid JSON, skipped", "topic", topic, "size", len(payload))
		return nil
	}

	printer.mutex.Lock()
	defer printer.mutex.Unlock()

	if printer.failed {
		return nil
	}

	if _, err := printer.output.Write(line); err != nil {
		printer.failed = true
		return err
	}

	return nil
}

// formatDumpLine builds one output line for a received message.
//
// Payload and topic come from whoever publishes, so they are never written as they are:
// control characters such as ESC or \r would be acted on by the terminal (and by anyone who
// later cats a saved dump).
//
//   - Default: strconv.QuoteToASCII, which writes printable ASCII as itself and everything else
//     as an escape (\x1b, \r, ‮, ...). strconv.Unquote gives back the exact bytes.
//   - JSON: one JSON object per line, {"topic":...,"payload":...}, with the payload embedded
//     as JSON. Valid JSON cannot contain raw control characters. A payload that is not valid
//     JSON returns an error and the message is skipped.
func formatDumpLine(topic string, payload []byte, options DumpOptions) ([]byte, error) {
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

	// Keep <, > and & as they are; the default turns them into < etc., which is only
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
