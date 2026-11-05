package mqttload

import (
	"sync"
	"testing"

	"github.com/HdrHistogram/hdrhistogram-go"
)

func newLatencyHistogram() *hdrhistogram.Histogram {
	return hdrhistogram.New(1, latencyHighestMicroseconds, 2)
}

func TestLatencyBucketsMatchHdrHistogram(t *testing.T) {
	if got := len(newLatencyHistogram().Export().Counts); got != latencyCountsLength {
		t.Fatalf("library counts length = %d, want %d", got, latencyCountsLength)
	}

	values := []int64{
		-5, 0, 1, 127, 128, 255, 256, 257, 1000, 12345, 1_500_000,
		3_599_999_999, 3_600_000_000, 3_600_000_001, 10_000_000_000,
	}

	expected := newLatencyHistogram()
	buckets := &latencyBuckets{}
	chunked := &chunkedLatencyBuckets{}

	for _, value := range values {
		buckets.record(value)
		chunked.record(value)

		if err := expected.RecordValue(min(max(value, 0), latencyHighestMicroseconds)); err != nil {
			t.Fatal(err)
		}
	}

	set := newLatencyBucketSet()
	set.forClient(3).record(1000)
	setExpected := newLatencyHistogram()
	_ = setExpected.RecordValue(1000)

	fromBuckets := newLatencyHistogram()
	buckets.addTo(fromBuckets)
	fromChunked := newLatencyHistogram()
	chunked.addTo(fromChunked)
	fromSet := newLatencyHistogram()
	set.addTo(fromSet)

	assertSameHistogram(t, "latencyBuckets", expected, fromBuckets)
	assertSameHistogram(t, "chunkedLatencyBuckets", expected, fromChunked)
	assertSameHistogram(t, "latencyBucketSet", setExpected, fromSet)
}

func assertSameHistogram(t *testing.T, name string, want, got *hdrhistogram.Histogram) {
	t.Helper()

	if want.TotalCount() != got.TotalCount() {
		t.Errorf("%s: TotalCount = %d, want %d", name, got.TotalCount(), want.TotalCount())
	}
	if want.Min() != got.Min() || want.Max() != got.Max() {
		t.Errorf("%s: Min/Max = %d/%d, want %d/%d", name, got.Min(), got.Max(), want.Min(), want.Max())
	}

	for _, quantile := range []float64{0, 50, 90, 99, 99.9, 100} {
		if want.ValueAtQuantile(quantile) != got.ValueAtQuantile(quantile) {
			t.Errorf("%s: p%v = %d, want %d", name, quantile, got.ValueAtQuantile(quantile), want.ValueAtQuantile(quantile))
		}
	}
}

func TestLatencyBucketsConcurrentRecording(t *testing.T) {
	const goroutines = 64
	const recordsPerGoroutine = 10_000

	buckets := &latencyBuckets{}
	chunked := &chunkedLatencyBuckets{}

	var waitGroup sync.WaitGroup
	for goroutine := range goroutines {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			for i := range recordsPerGoroutine {
				value := int64(goroutine*recordsPerGoroutine+i) * 7
				buckets.record(value)
				chunked.record(value)
			}
		}()
	}
	waitGroup.Wait()

	const want = goroutines * recordsPerGoroutine

	fromBuckets := newLatencyHistogram()
	buckets.addTo(fromBuckets)
	if fromBuckets.TotalCount() != want {
		t.Errorf("latencyBuckets TotalCount = %d, want %d", fromBuckets.TotalCount(), want)
	}

	fromChunked := newLatencyHistogram()
	chunked.addTo(fromChunked)
	if fromChunked.TotalCount() != want {
		t.Errorf("chunkedLatencyBuckets TotalCount = %d, want %d", fromChunked.TotalCount(), want)
	}
}

func BenchmarkLatencyBucketsRecord(b *testing.B) {
	buckets := &latencyBuckets{}

	b.Run("single", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buckets.record(int64(i&0xFFFFF) + 1000)
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				buckets.record(int64(i&0xFFFFF) + 1000)
				i++
			}
		})
	})
}

func BenchmarkChunkedLatencyBucketsRecord(b *testing.B) {
	buckets := &chunkedLatencyBuckets{}

	b.Run("single", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buckets.record(int64(i&0xFFFFF) + 1000)
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				buckets.record(int64(i&0xFFFFF) + 1000)
				i++
			}
		})
	})
}
