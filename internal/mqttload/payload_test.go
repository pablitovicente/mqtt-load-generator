package mqttload

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStaticPayloadGenerator_ExactSize(t *testing.T) {
	tests := []int{0, 1, 100, 4096}

	for _, size := range tests {
		t.Run(fmt.Sprintf("size %d", size), func(t *testing.T) {
			generate := newPayloadGenerator(size, false)

			payload := generate()
			if len(payload) != size {
				t.Errorf("len(payload) = %d, want %d", len(payload), size)
			}
		})
	}
}

// TestStaticPayloadGenerator_ReusesSamePayload checks that the default payload is built once
// and reused for every message, like v1, instead of being regenerated each call.
func TestStaticPayloadGenerator_ReusesSamePayload(t *testing.T) {
	generate := newPayloadGenerator(64, false)

	first := generate()
	second := generate()

	if len(first) != 64 || len(second) != 64 {
		t.Fatalf("expected 64-byte payloads, got %d and %d", len(first), len(second))
	}

	// Same underlying array: mutating one through the returned slice would show up in the
	// other, proving the generator hands back the same bytes rather than fresh random ones.
	first[0] = first[0] + 1
	if first[0] != second[0] {
		t.Errorf("expected the same payload to be reused, got different bytes after mutation")
	}
}

// benchmarkPayload is the v1 format string, spelled out here so the test fails loudly if
// anyone changes the frozen format by accident.
func benchmarkPayload(timestamp int64, padding string) string {
	return fmt.Sprintf(`{"timestamp":%d,"padding":"%s"}`, timestamp, padding)
}

func TestBenchmarkPayloadGenerator_MatchesV1Format(t *testing.T) {
	generate := benchmarkPayloadGenerator(100)

	before := time.Now().UnixMilli()
	payload := generate()
	after := time.Now().UnixMilli()

	var decoded struct {
		Timestamp int64  `json:"timestamp"`
		Padding   string `json:"padding"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v, payload: %s", err, payload)
	}

	if decoded.Timestamp < before || decoded.Timestamp > after {
		t.Errorf("timestamp %d not within [%d, %d]", decoded.Timestamp, before, after)
	}

	wantPaddingLength := 100 - 50
	if len(decoded.Padding) != wantPaddingLength {
		t.Errorf("padding length = %d, want %d", len(decoded.Padding), wantPaddingLength)
	}
	if decoded.Padding != strings.Repeat("x", wantPaddingLength) {
		t.Errorf("padding = %q, want %d x's", decoded.Padding, wantPaddingLength)
	}

	// Byte-for-byte against v1's own format string, keys in the same order, for the exact
	// timestamp this generator produced.
	want := benchmarkPayload(decoded.Timestamp, decoded.Padding)
	if string(payload) != want {
		t.Errorf("payload = %q, want %q", payload, want)
	}
}

// TestBenchmarkPayloadGenerator_FreshTimestampPerCall checks that, unlike the default payload,
// the benchmark payload is rebuilt (with a fresh timestamp) on every call.
func TestBenchmarkPayloadGenerator_FreshTimestampPerCall(t *testing.T) {
	generate := benchmarkPayloadGenerator(100)

	first := generate()
	time.Sleep(2 * time.Millisecond)
	second := generate()

	if string(first) == string(second) {
		t.Errorf("expected different payloads (different timestamps), got the same: %s", first)
	}
}

func TestBenchmarkPayloadGenerator_SizeBelow50ClampsToEmptyPadding(t *testing.T) {
	tests := []int{0, 1, 49, 50}

	for _, size := range tests {
		t.Run(fmt.Sprintf("size %d", size), func(t *testing.T) {
			generate := benchmarkPayloadGenerator(size)
			payload := generate()

			var decoded struct {
				Padding string `json:"padding"`
			}
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("payload is not valid JSON: %v, payload: %s", err, payload)
			}

			if decoded.Padding != "" {
				t.Errorf("expected empty padding for size %d, got %q", size, decoded.Padding)
			}
		})
	}
}

// BenchmarkBenchmarkPayload measures building one --benchmark payload, the work done once per
// message. Run with: go test ./internal/mqttload -run '^$' -bench BenchmarkPayload
//
// Measured when this was written (2026-09-25): about 150 ns and 1 allocation for 100 bytes,
// against about 375 ns and 4 allocations for v1's fmt.Sprintf version.
func BenchmarkBenchmarkPayload(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("size %d", size), func(b *testing.B) {
			generate := benchmarkPayloadGenerator(size)

			b.ReportAllocs()
			for b.Loop() {
				generate()
			}
		})
	}
}
