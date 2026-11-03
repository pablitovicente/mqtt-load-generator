package mqttload

import (
	"math/bits"
	"runtime"
	"sync/atomic"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// The latency counters use the exact bucket layout of an hdrhistogram.Histogram created with
// New(1, 3_600_000_000, 2): values in microseconds, one hour at most, 2 significant digits.
// That makes 256 sub-buckets per power of two (128 once past the first one) and 3328 counters.
// The index math below is copied from hdrhistogram-go (MIT license, hdr.go: getBucketIndex,
// getSubBucketIdx, countsIndex, valueFromIndex), fixed to this one geometry.
const (
	latencyHighestMicroseconds = 3_600_000_000
	latencySubBucketHalfCount  = 128
	latencySubBucketHalfShift  = 7
	latencyCountsLength        = 3328
)

// latencyCountsIndex returns the counter position for a latency in microseconds. Negative
// values count as 0 and values over one hour count as one hour.
func latencyCountsIndex(microseconds int64) int {
	microseconds = min(max(microseconds, 0), latencyHighestMicroseconds)

	// The first bucket holds values up to 255. Each next bucket covers twice the range.
	bucketIndex := 64 - bits.LeadingZeros64(uint64(microseconds|255)) - 8
	subBucketIndex := int(microseconds >> uint(bucketIndex))

	return (bucketIndex+1)<<latencySubBucketHalfShift + subBucketIndex - latencySubBucketHalfCount
}

// latencyValueForIndex returns the lowest microsecond value that lands on a counter position.
func latencyValueForIndex(countsIndex int) int64 {
	if countsIndex < latencySubBucketHalfCount {
		return int64(countsIndex)
	}

	bucketIndex := countsIndex>>latencySubBucketHalfShift - 1
	subBucketIndex := countsIndex&(latencySubBucketHalfCount-1) + latencySubBucketHalfCount

	return int64(subBucketIndex) << uint(bucketIndex)
}

// latencyBuckets is one dense array of counters, safe for many goroutines to record into.
type latencyBuckets struct {
	counts [latencyCountsLength]atomic.Uint32
}

func (buckets *latencyBuckets) record(microseconds int64) {
	buckets.counts[latencyCountsIndex(microseconds)].Add(1)
}

// addTo adds every count into the histogram. Call it only after all recording has finished.
func (buckets *latencyBuckets) addTo(histogram *hdrhistogram.Histogram) {
	for countsIndex := range buckets.counts {
		count := buckets.counts[countsIndex].Load()
		if count == 0 {
			continue
		}

		_ = histogram.RecordValues(latencyValueForIndex(countsIndex), int64(count))
	}
}

// latencyBucketSet is a fixed number of latencyBuckets shared by all clients, so memory does
// not grow with the client count. Clients that share one latencyBuckets record into the same
// counters with atomic adds.
type latencyBucketSet struct {
	buckets []*latencyBuckets
}

func newLatencyBucketSet() *latencyBucketSet {
	// Four per CPU, rounded up to a power of two, between 64 and 256.
	size := 64
	for size < 4*runtime.GOMAXPROCS(0) && size < 256 {
		size *= 2
	}

	set := &latencyBucketSet{buckets: make([]*latencyBuckets, size)}
	for i := range set.buckets {
		set.buckets[i] = &latencyBuckets{}
	}

	return set
}

// forClient returns the latencyBuckets a client records into. Call it once per client, not
// once per message.
func (set *latencyBucketSet) forClient(clientNumber int) *latencyBuckets {
	return set.buckets[clientNumber%len(set.buckets)]
}

// addTo adds every set member's counts into the histogram. Call it only after all recording
// has finished.
func (set *latencyBucketSet) addTo(histogram *hdrhistogram.Histogram) {
	for _, buckets := range set.buckets {
		buckets.addTo(histogram)
	}
}

// latencyChunk holds the counters for 128 neighbouring counter positions.
type latencyChunk [latencySubBucketHalfCount]atomic.Uint32

// chunkedLatencyBuckets is one client's counters, with memory allocated only for the ranges
// that were hit. Every chunk covers 128 positions (countsIndex / 128). That lines up with the
// library layout: the first bucket takes chunks 0 and 1 (values 0..255), and every later
// bucket takes exactly one chunk (its upper 128 sub-buckets).
type chunkedLatencyBuckets struct {
	chunks [latencyCountsLength / latencySubBucketHalfCount]atomic.Pointer[latencyChunk]
}

func (buckets *chunkedLatencyBuckets) record(microseconds int64) {
	countsIndex := latencyCountsIndex(microseconds)
	chunkSlot := &buckets.chunks[countsIndex>>latencySubBucketHalfShift]

	chunk := chunkSlot.Load()
	if chunk == nil {
		// If another goroutine installed a chunk first, drop ours and use theirs.
		newChunk := &latencyChunk{}
		if chunkSlot.CompareAndSwap(nil, newChunk) {
			chunk = newChunk
		} else {
			chunk = chunkSlot.Load()
		}
	}

	chunk[countsIndex&(latencySubBucketHalfCount-1)].Add(1)
}

// addTo adds every count into the histogram. Call it only after all recording has finished.
func (buckets *chunkedLatencyBuckets) addTo(histogram *hdrhistogram.Histogram) {
	for chunkNumber := range buckets.chunks {
		chunk := buckets.chunks[chunkNumber].Load()
		if chunk == nil {
			continue
		}

		for offset := range chunk {
			count := chunk[offset].Load()
			if count == 0 {
				continue
			}

			countsIndex := chunkNumber<<latencySubBucketHalfShift + offset
			_ = histogram.RecordValues(latencyValueForIndex(countsIndex), int64(count))
		}
	}
}
