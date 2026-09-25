package display

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestFormatDumpLine(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		payload string
		options DumpOptions
		want    string
	}{
		{"plain text", "load/test", "hello", DumpOptions{}, "\"hello\"\n"},
		{"benchmark JSON", "load/test", `{"timestamp":1,"padding":"xx"}`, DumpOptions{}, `"{\"timestamp\":1,\"padding\":\"xx\"}"` + "\n"},
		{"escape sequence", "load/test", "\x1b[2Jboom", DumpOptions{}, `"\x1b[2Jboom"` + "\n"},
		{"carriage return and newline", "load/test", "a\rb\nc", DumpOptions{}, `"a\rb\nc"` + "\n"},
		{"bidi override", "load/test", "abc\u202edef", DumpOptions{}, `"abc\u202edef"` + "\n"},
		{"invalid UTF-8", "load/test", "\xff\xfe", DumpOptions{}, `"\xff\xfe"` + "\n"},
		{"empty payload", "load/test", "", DumpOptions{}, "\"\"\n"},
		{"show topic", "load/test", "hello", DumpOptions{ShowTopic: true}, "\"load/test\"\t\"hello\"\n"},
		{"topic with escape sequence", "load/\x1b[31m", "x", DumpOptions{ShowTopic: true}, `"load/\x1b[31m"` + "\t\"x\"\n"},
		{"json embeds payload", "load/test", `{"timestamp":1, "padding":"xx"}`, DumpOptions{JSON: true}, `{"topic":"load/test","payload":{"timestamp":1,"padding":"xx"}}` + "\n"},
		{"json keeps key order", "load/test", `{"b":1,"a":2}`, DumpOptions{JSON: true}, `{"topic":"load/test","payload":{"b":1,"a":2}}` + "\n"},
		{"json keeps < > &", "load/test", `{"html":"<b>&</b>"}`, DumpOptions{JSON: true}, `{"topic":"load/test","payload":{"html":"<b>&</b>"}}` + "\n"},
		{"json ignores show topic", "load/test", `1`, DumpOptions{JSON: true, ShowTopic: true}, `{"topic":"load/test","payload":1}` + "\n"},
		{"json escapes control characters in the topic", "load/\x1b", `1`, DumpOptions{JSON: true}, `{"topic":"load/\u001b","payload":1}` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatDumpLine(tt.topic, []byte(tt.payload), tt.options)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("formatDumpLine = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatDumpLine_QuotedOutputGivesBackExactBytes checks that the default format loses
// nothing: strconv.Unquote returns the original payload, byte for byte.
func TestFormatDumpLine_QuotedOutputGivesBackExactBytes(t *testing.T) {
	payload := []byte{0x00, 0x1b, '[', '2', 'J', 0xff, '\r', '\n', 0xe2, 0x80, 0xae, 'x'}

	line, err := formatDumpLine("load/test", payload, DumpOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unquoted, err := strconv.Unquote(strings.TrimSuffix(string(line), "\n"))
	if err != nil {
		t.Fatalf("strconv.Unquote: %v", err)
	}
	if !bytes.Equal([]byte(unquoted), payload) {
		t.Errorf("unquoted = %q, want %q", unquoted, payload)
	}
}

func TestFormatDumpLine_JSONRejectsInvalidPayloads(t *testing.T) {
	for _, payload := range []string{"not json", `{"open":`, "", "\x1b[2J"} {
		t.Run(fmt.Sprintf("%q", payload), func(t *testing.T) {
			if _, err := formatDumpLine("load/test", []byte(payload), DumpOptions{JSON: true}); err == nil {
				t.Error("expected an error, got none")
			}
		})
	}
}

func TestMessagePrinter_PrintWritesFormattedLine(t *testing.T) {
	var output bytes.Buffer
	printer := NewMessagePrinter(&output, discardLogger(), DumpOptions{})

	if err := printer.Print("load/test", []byte("hello")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.String() != "\"hello\"\n" {
		t.Errorf("output = %q, want %q", output.String(), "\"hello\"\n")
	}
}

func TestMessagePrinter_ShowTopicPrintsTopicAndTab(t *testing.T) {
	var output bytes.Buffer
	printer := NewMessagePrinter(&output, discardLogger(), DumpOptions{ShowTopic: true})

	if err := printer.Print("load/test", []byte("payload")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "\"load/test\"\t\"payload\"\n"
	if output.String() != want {
		t.Errorf("output = %q, want %q", output.String(), want)
	}
}

// TestMessagePrinter_JSONSkipsInvalidPayloadsWithWarning checks that with --json a payload that
// is not valid JSON is not written, a warning is logged without the payload, and the next valid
// message still comes through.
func TestMessagePrinter_JSONSkipsInvalidPayloadsWithWarning(t *testing.T) {
	var output bytes.Buffer
	logger, logs := captureLogger()
	printer := NewMessagePrinter(&output, logger, DumpOptions{JSON: true})

	if err := printer.Print("load/test", []byte("secret-not-json")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := printer.Print("load/test", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := `{"topic":"load/test","payload":{"ok":true}}` + "\n"
	if output.String() != want {
		t.Errorf("output = %q, want %q", output.String(), want)
	}

	if !strings.Contains(logs.String(), "payload is not valid JSON") {
		t.Errorf("expected a warning in the logs, got: %s", logs.String())
	}
	if strings.Contains(logs.String(), "secret-not-json") {
		t.Errorf("the warning must not include the payload, got: %s", logs.String())
	}
}

// TestMessagePrinter_ConcurrentPrintsProduceWholeLines delivers many payloads from many
// goroutines at once, the way paho's own callback goroutines would, and checks that every line
// in output is exactly one of the payloads sent -- never a mix of two. Run with -race: output is
// a plain bytes.Buffer, not safe for concurrent use on its own, so a missing or broken lock in
// messagePrinter would show up either as a reported data race or as a corrupted line.
func TestMessagePrinter_ConcurrentPrintsProduceWholeLines(t *testing.T) {
	var output bytes.Buffer
	printer := NewMessagePrinter(&output, discardLogger(), DumpOptions{})

	const goroutineCount = 100

	payloads := make([]string, goroutineCount)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("payload-%03d-with-extra-padding-so-a-partial-write-would-show", i)
	}

	var waitGroup sync.WaitGroup
	for _, payload := range payloads {
		waitGroup.Add(1)
		go func(payload string) {
			defer waitGroup.Done()
			if err := printer.Print("load/test", []byte(payload)); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}(payload)
	}
	waitGroup.Wait()

	trimmed := strings.TrimRight(output.String(), "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) != goroutineCount {
		t.Fatalf("expected %d lines, got %d: %q", goroutineCount, len(lines), output.String())
	}

	want := make(map[string]bool, goroutineCount)
	for _, payload := range payloads {
		want[strconv.QuoteToASCII(payload)] = true
	}
	for _, line := range lines {
		if !want[line] {
			t.Errorf("unexpected or corrupted line: %q", line)
		}
	}
}

// failingWriter is an io.Writer that always fails, like stdout on a full disk.
type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("disk full")
}

// TestMessagePrinter_StopsWritingAfterFirstError checks that once a write fails, later Print
// calls return nil instead of trying (and failing) again, and don't panic.
func TestMessagePrinter_StopsWritingAfterFirstError(t *testing.T) {
	printer := NewMessagePrinter(failingWriter{}, discardLogger(), DumpOptions{})

	err := printer.Print("load/test", []byte("first"))
	if err == nil {
		t.Fatal("expected an error from the first write")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("expected the write error, got %v", err)
	}

	if err := printer.Print("load/test", []byte("second")); err != nil {
		t.Errorf("expected no error once the printer has already failed once, got %v", err)
	}
}
