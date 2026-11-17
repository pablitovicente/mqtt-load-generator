package mqttload

import (
	"bytes"
	"crypto/rand"
	"strconv"
	"strings"
	"time"
)

// payloadGenerator returns the next payload to publish.
type payloadGenerator func() []byte

// newPayloadGenerator builds the payload generator for one client, following --size,
// --benchmark and the payload rendered from --json-template (nil without it).
func newPayloadGenerator(size int, benchmark bool, templatePayload []byte) payloadGenerator {
	switch {
	case templatePayload != nil && benchmark:
		return timestampPayloadGenerator(afterTimestamp(templatePayload))
	case templatePayload != nil:
		// Nothing ever writes to these bytes, so every message can send the same slice, like
		// the static payload.
		return func() []byte {
			return templatePayload
		}
	case benchmark:
		return benchmarkPayloadGenerator(size)
	default:
		return staticPayloadGenerator(size)
	}
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

	return timestampPayloadGenerator([]byte(`,"padding":"` + padding + `"}`))
}

// timestampPayloadGenerator builds a generator that returns, for every call, a new slice with
// {"timestamp":, the current time in Unix milliseconds, then rest. rest must finish the JSON
// object, for example ,"padding":"xxx"} or }.
func timestampPayloadGenerator(rest []byte) payloadGenerator {
	const start = `{"timestamp":` // opens the object, up to the timestamp value

	// Room for the fixed text, rest, and the longest possible timestamp (an int64 has at most
	// 20 characters), so append never has to grow the slice.
	capacity := len(start) + 20 + len(rest)

	return func() []byte {
		payload := make([]byte, 0, capacity)

		payload = append(payload, start...)
		payload = strconv.AppendInt(payload, time.Now().UnixMilli(), 10) // milliseconds since 1970, in decimal
		payload = append(payload, rest...)

		return payload
	}
}

// afterTimestamp returns what follows the timestamp when --benchmark adds it to a payload
// rendered from --json-template: the object without its opening {, after a comma, or just the
// closing } for an empty object. CheckJSONTemplateForBenchmark has already checked that the
// payload is a JSON object.
func afterTimestamp(templatePayload []byte) []byte {
	const whitespace = " \t\r\n"

	// Drop the opening {.
	inside := bytes.TrimLeft(templatePayload, whitespace)[1:]

	if bytes.HasPrefix(bytes.TrimLeft(inside, whitespace), []byte("}")) {
		return bytes.TrimLeft(inside, whitespace)
	}

	return append([]byte(","), inside...)
}
