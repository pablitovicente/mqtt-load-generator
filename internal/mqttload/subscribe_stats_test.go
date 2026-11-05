package mqttload

import (
	"strings"
	"testing"
)

func TestBenchmarkTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		wantValue int64
		wantOK    bool
	}{
		{"valid, comma after digits", []byte(`{"timestamp":1790288219472,"padding":"xx"}`), 1790288219472, true},
		{"valid, brace after digits", []byte(`{"timestamp":42}`), 42, true},
		{"from the pub generator", newPayloadGenerator(100, true)(), 0, true},
		{"missing prefix", []byte(`{"time":123,"padding":""}`), 0, false},
		{"empty payload", nil, 0, false},
		{"no digits", []byte(`{"timestamp":,"padding":""}`), 0, false},
		{"non-digit after digits", []byte(`{"timestamp":12x3,"padding":""}`), 0, false},
		{"no end after digits", []byte(`{"timestamp":123`), 0, false},
		{"too many digits", []byte(`{"timestamp":` + strings.Repeat("1", 20) + `}`), 0, false},
		{"19 digits over the int64 limit", []byte(`{"timestamp":9999999999999999999}`), 0, false},
		{"random bytes", []byte{0x00, 0xff, 0x7b, 0x22, 0x13}, 0, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, ok := benchmarkTimestamp(test.payload)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}

			// The generator's timestamp is the current time, so only check the others exactly.
			if test.name == "from the pub generator" {
				if value <= 0 {
					t.Errorf("value = %d, want a positive timestamp", value)
				}
				return
			}

			if value != test.wantValue {
				t.Errorf("value = %d, want %d", value, test.wantValue)
			}
		})
	}
}
