package mqttload

import (
	"crypto/rand"
	"strconv"
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
// their own collectors. Only the timestamp changes between calls:
//
//	{"timestamp":1790288219472,"padding":"xxxx...x"}
//
// The bytes are built by hand instead of with fmt.Sprintf because this runs once per message:
// Sprintf plus the []byte conversion made 4 allocations per message, this makes 1, and it is
// about 2.3 times faster (see BenchmarkBenchmarkPayload). v1 built the same bytes with
// fmt.Sprintf(`{"timestamp":%d,"padding":"%s"}`, ...).
func benchmarkPayloadGenerator(size int) payloadGenerator {
	// v1 subtracted 50 for the JSON around the padding, and clamped at 0.
	paddingSize := size - 50
	if paddingSize < 0 {
		paddingSize = 0
	}
	padding := strings.Repeat("x", paddingSize)

	const (
		start  = `{"timestamp":` // opens the object, up to the timestamp value
		middle = `,"padding":"`  // between the timestamp value and the padding
		end    = `"}`            // closes the padding string and the object
	)

	// Room for the fixed text, the padding, and the longest possible timestamp (an int64 has
	// at most 20 characters), so append never has to grow the slice.
	capacity := len(start) + 20 + len(middle) + len(padding) + len(end)

	return func() []byte {
		payload := make([]byte, 0, capacity)

		payload = append(payload, start...)
		payload = strconv.AppendInt(payload, time.Now().UnixMilli(), 10) // milliseconds since 1970, in decimal
		payload = append(payload, middle...)
		payload = append(payload, padding...)
		payload = append(payload, end...)

		return payload
	}
}
