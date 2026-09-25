package mqttload

import (
	"sync/atomic"
	"time"
)

// PublishSnapshot is a plain-value snapshot of a publish run's progress, for display to read.
type PublishSnapshot struct {
	ClientsConnected int
	ClientsTotal     int

	Published int64
	Acked     int64
	Failed    int64
	TimedOut  int64

	// PublishStartedAt is zero until publishing actually starts. It stays zero when
	// connecting fails or is cancelled: the run never gets to the publishing phase.
	PublishStartedAt time.Time

	// FinishedAt is zero until the whole run is over.
	FinishedAt time.Time
}

// PublishProgress is what RunPublish reports as it works, and what display reads on its own
// timer through Snapshot. Safe for concurrent use: RunPublish updates it from one goroutine per
// client, plus the connect step, while display reads it from another.
type PublishProgress struct {
	clientsTotal int
	counters     []*publishCounters

	clientsConnected atomic.Int64
	publishStartedAt atomicTime
	finishedAt       atomicTime

	// connectFinished closes once every client has connected, or connecting has failed or
	// been cancelled.
	connectFinished chan struct{}

	// done closes once the whole run is over, publishing or not.
	done chan struct{}
}

// NewPublishProgress creates a progress value for a run of clientsTotal clients.
func NewPublishProgress(clientsTotal int) *PublishProgress {
	counters := make([]*publishCounters, clientsTotal)
	for i := range counters {
		counters[i] = &publishCounters{}
	}

	return &PublishProgress{
		clientsTotal:    clientsTotal,
		counters:        counters,
		connectFinished: make(chan struct{}),
		done:            make(chan struct{}),
	}
}

// ConnectFinished closes once every client has connected, or connecting has failed or been
// cancelled.
func (progress *PublishProgress) ConnectFinished() <-chan struct{} {
	return progress.connectFinished
}

// Done closes once the whole run is over.
func (progress *PublishProgress) Done() <-chan struct{} {
	return progress.done
}

// clientConnected records that one more client connected.
func (progress *PublishProgress) clientConnected() {
	progress.clientsConnected.Add(1)
}

// counterFor returns the counter one client should add its own results to, numbered
// 0..clientsTotal-1.
func (progress *PublishProgress) counterFor(clientIndex int) *publishCounters {
	return progress.counters[clientIndex]
}

// finishConnecting marks connecting as over. started is true once every client has connected
// and publishing is about to begin; false when connecting failed or was cancelled, in which
// case the caller also calls finish immediately after, since there is nothing left to run.
func (progress *PublishProgress) finishConnecting(started bool) {
	if started {
		progress.publishStartedAt.set(time.Now())
	}
	close(progress.connectFinished)
}

// finish marks the whole run as over.
func (progress *PublishProgress) finish() {
	progress.finishedAt.set(time.Now())
	close(progress.done)
}

// Snapshot returns a plain-value copy of the current progress.
func (progress *PublishProgress) Snapshot() PublishSnapshot {
	summary := sumPublishCounters(progress.counters)

	return PublishSnapshot{
		ClientsConnected: int(progress.clientsConnected.Load()),
		ClientsTotal:     progress.clientsTotal,
		Published:        summary.Published,
		Acked:            summary.Acked,
		Failed:           summary.Failed,
		TimedOut:         summary.TimedOut,
		PublishStartedAt: progress.publishStartedAt.load(),
		FinishedAt:       progress.finishedAt.load(),
	}
}
