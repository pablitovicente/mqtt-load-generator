package mqttload

import (
	"testing"
	"time"
)

// TestSampleTotal_RecordsFullIntervalsAndStops checks that the sampler records the acks of each
// interval and returns once the run is over.
func TestSampleTotal_RecordsFullIntervalsAndStops(t *testing.T) {
	progress := NewPublishProgress(1)
	stats := NewPublishStats(1, false)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		stats.ackedPerInterval = sampleTotal(func() int64 { return progress.Snapshot().Acked }, progress.Done(), 10*time.Millisecond)
	}()

	progress.counterFor(0).acked.Add(5)
	time.Sleep(55 * time.Millisecond)
	progress.finish()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("sampler did not stop after the run finished")
	}

	if len(stats.ackedPerInterval) < 2 {
		t.Fatalf("recorded %d intervals, want at least 2", len(stats.ackedPerInterval))
	}

	var total int64
	for _, acked := range stats.ackedPerInterval {
		total += acked
	}
	if total != 5 {
		t.Errorf("intervals add up to %d acks, want 5", total)
	}
}
