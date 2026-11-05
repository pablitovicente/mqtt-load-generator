package mqttload

import (
	"math"
	"sync/atomic"
)

// benchmarkTimestampPrefix is how every pub --benchmark payload starts.
const benchmarkTimestampPrefix = `{"timestamp":`

// maxTimestampDigits is the most digits a timestamp may have. 19 digits is the longest number
// an int64 can hold.
const maxTimestampDigits = 19

// SubscribeStats collects detailed numbers during a subscribe run. Read it only after
// RunSubscribe has returned.
type SubscribeStats struct {
	// benchmark is true when --benchmark payloads are read for their timestamp.
	benchmark bool

	// latency holds the end-to-end latencies in milliseconds. It is safe for many goroutines
	// to record into.
	latency *latencyBucketSet

	// withoutTimestamp counts messages that did not start with a benchmark timestamp.
	withoutTimestamp atomic.Int64

	// negativeLatency counts messages whose timestamp is later than the time they arrived,
	// which means the publisher's and subscriber's clocks are not in sync.
	negativeLatency atomic.Int64

	// receivedPerInterval is the number of messages received in each full ackSampleInterval.
	// Only the sampler goroutine writes it.
	receivedPerInterval []int64
}

// NewSubscribeStats creates the stats for one subscribe run. benchmark turns on reading the
// timestamp from every payload.
func NewSubscribeStats(benchmark bool) *SubscribeStats {
	return &SubscribeStats{
		benchmark: benchmark,
		latency:   newLatencyBucketSet(),
	}
}

// benchmarkTimestamp reads the timestamp (milliseconds since 1970) from a pub --benchmark
// payload. The payload must start with {"timestamp":, followed by up to 19 digits and then a
// comma or a closing brace. It scans the bytes directly, so it allocates nothing.
func benchmarkTimestamp(payload []byte) (int64, bool) {
	if len(payload) < len(benchmarkTimestampPrefix) || string(payload[:len(benchmarkTimestampPrefix)]) != benchmarkTimestampPrefix {
		return 0, false
	}

	var value uint64
	digitCount := 0

	for _, character := range payload[len(benchmarkTimestampPrefix):] {
		if character >= '0' && character <= '9' {
			digitCount++
			if digitCount > maxTimestampDigits {
				return 0, false
			}

			value = value*10 + uint64(character-'0')

			continue
		}

		if (character != ',' && character != '}') || digitCount == 0 || value > math.MaxInt64 {
			return 0, false
		}

		return int64(value), true
	}

	// The payload ended before a comma or brace.
	return 0, false
}
