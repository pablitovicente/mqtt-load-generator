package mqttload

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// payloadGenerator returns the next payload to publish.
type payloadGenerator func() []byte

// newPayloadGenerator builds the payload generator for one client, following --size and
// --benchmark.
func newPayloadGenerator(size int, benchmark bool) payloadGenerator {
	if benchmark {
		return benchmarkPayloadGenerator(size)
	}
	return staticPayloadGenerator(size)
}

// staticPayloadGenerator builds size random bytes once, and returns a generator that hands back
// those same bytes for every call, exactly like v1.
func staticPayloadGenerator(size int) payloadGenerator {
	payload := make([]byte, size)

	// Since Go 1.24, crypto/rand.Read never returns an error: it crashes the program instead.
	rand.Read(payload)

	return func() []byte {
		return payload
	}
}

// benchmarkPayloadGenerator builds a generator for the --benchmark payload. This format is
// frozen: it must stay byte-for-byte what v1 produced, because teams parse the timestamp in
// their own collectors. Only the timestamp changes between calls.
func benchmarkPayloadGenerator(size int) payloadGenerator {
	paddingSize := size - 50
	if paddingSize < 0 {
		paddingSize = 0
	}
	padding := strings.Repeat("x", paddingSize)

	return func() []byte {
		return []byte(fmt.Sprintf(`{"timestamp":%d,"padding":"%s"}`, time.Now().UnixMilli(), padding))
	}
}
