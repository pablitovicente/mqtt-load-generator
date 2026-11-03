package mqttload

import "time"

// ackSampleInterval is how often the sampler records how many publishes were acked.
const ackSampleInterval = time.Second

// clientStats is what one client collects for the final report. Every field is written by one
// goroutine only and read after the run has finished, so none of it needs a lock.
type clientStats struct {
	clientID string

	// connectDuration is written by the goroutine that connects this client.
	connectDuration time.Duration

	// inflightWait is the total time spent waiting for a free in-flight slot. Only the
	// client's sending goroutine writes it.
	inflightWait time.Duration

	// finishedAt is set once all of the client's outstanding publishes have finished, just
	// before it disconnects.
	finishedAt time.Time

	// sharedLatency is picked once from the shared set. perClientLatency is nil unless
	// per-client stats were asked for. Both are safe for many goroutines to record into.
	sharedLatency    *latencyBuckets
	perClientLatency *chunkedLatencyBuckets
}

// PublishStats collects detailed numbers during a publish run. Read it only after RunPublish
// has returned.
type PublishStats struct {
	latency *latencyBucketSet
	clients []*clientStats

	// ackedPerInterval is the number of acks in each full ackSampleInterval. Only the sampler
	// goroutine writes it.
	ackedPerInterval []int64
}

// NewPublishStats creates the stats for a run of clients clients. perClientStats makes every
// client keep its own latency counters on top of the shared ones.
func NewPublishStats(clients int, perClientStats bool) *PublishStats {
	stats := &PublishStats{
		latency: newLatencyBucketSet(),
		clients: make([]*clientStats, clients),
	}

	for i := range stats.clients {
		stats.clients[i] = &clientStats{sharedLatency: stats.latency.forClient(i)}

		if perClientStats {
			stats.clients[i].perClientLatency = &chunkedLatencyBuckets{}
		}
	}

	return stats
}

// sampleTotal reads currentTotal every interval until done closes, and returns how much the
// total grew in each full interval. A last, partly finished interval is dropped. pub samples
// acks and sub samples received messages, both from the counters the progress display reads.
func sampleTotal(currentTotal func() int64, done <-chan struct{}, interval time.Duration) []int64 {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var growthPerInterval []int64
	var previousTotal int64

	for {
		select {
		case <-ticker.C:
			total := currentTotal()
			growthPerInterval = append(growthPerInterval, total-previousTotal)
			previousTotal = total
		case <-done:
			return growthPerInterval
		}
	}
}
