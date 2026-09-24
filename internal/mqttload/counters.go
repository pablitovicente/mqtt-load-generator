package mqttload

import "sync/atomic"

// publishCounters counts what one client's publish loop did. Plain atomics, so the goroutine
// that waits for each publish's result and the reporting loop that samples them never need a
// lock between them.
//
// At QoS 0 there is no acknowledgement from the broker: the token completes as soon as the
// message is written, so Acked there means "sent", not "confirmed by the broker".
type publishCounters struct {
	published atomic.Int64
	acked     atomic.Int64
	failed    atomic.Int64
	timedOut  atomic.Int64
}

// publishSummary is a plain-value snapshot of one or more publishCounters, added together. A
// later stats package can take over building and printing this; for now RunPublish only needs
// the sum.
type publishSummary struct {
	Published int64
	Acked     int64
	Failed    int64
	TimedOut  int64
}

// sumPublishCounters adds every client's counters together.
func sumPublishCounters(counters []*publishCounters) publishSummary {
	var summary publishSummary

	for _, counter := range counters {
		summary.Published += counter.published.Load()
		summary.Acked += counter.acked.Load()
		summary.Failed += counter.failed.Load()
		summary.TimedOut += counter.timedOut.Load()
	}

	return summary
}
