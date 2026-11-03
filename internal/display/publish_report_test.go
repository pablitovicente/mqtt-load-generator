package display

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pablitovicente/mqtt-load-generator/v2/internal/mqttload"
)

// TestWriteReport checks that the report is exactly one line of valid JSON, and that
// the ack latency key is left out when there is no ack latency.
func TestWriteReport(t *testing.T) {
	var output bytes.Buffer

	report := mqttload.PublishReport{}
	if err := WriteReport(&output, report); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	text := output.String()
	if !strings.HasSuffix(text, "\n") || strings.Count(text, "\n") != 1 {
		t.Fatalf("output is not exactly one line: %q", text)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	for _, key := range []string{"run", "settings", "messages", "acksPerInterval",
		"connectLatencyMicroseconds", "inflightWait", "perClientAckedPerSecond"} {
		if _, found := decoded[key]; !found {
			t.Errorf("missing key %q", key)
		}
	}
	if _, found := decoded["ackLatencyMicroseconds"]; found {
		t.Error("ackLatencyMicroseconds is present, want it left out")
	}
}

// TestWriteReportSubscribe checks that a sub report is also one line of valid JSON, and that
// the end-to-end latency key is left out when there is no latency.
func TestWriteReportSubscribe(t *testing.T) {
	var output bytes.Buffer

	if err := WriteReport(&output, mqttload.SubscribeReport{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	text := output.String()
	if !strings.HasSuffix(text, "\n") || strings.Count(text, "\n") != 1 {
		t.Fatalf("output is not exactly one line: %q", text)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	for _, key := range []string{"run", "settings", "messages", "receivedPerInterval"} {
		if _, found := decoded[key]; !found {
			t.Errorf("missing key %q", key)
		}
	}
	if _, found := decoded["endToEndLatencyMilliseconds"]; found {
		t.Error("endToEndLatencyMilliseconds is present, want it left out")
	}
}
