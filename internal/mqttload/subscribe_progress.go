package mqttload

import (
	"sync/atomic"
	"time"
)

// SubscribeSnapshot is a plain-value snapshot of a subscribe run's progress, for display to
// read.
type SubscribeSnapshot struct {
	Received int64

	// LastMessageAt is zero until the first message arrives.
	LastMessageAt time.Time

	// SubscribedAt is zero until the broker acknowledges the subscribe request. It stays zero
	// when the subscribe never succeeds, so display can tell a run that actually started from
	// one that never got going.
	SubscribedAt time.Time
}

// SubscribeProgress is what RunSubscribe reports as it works, and what display reads on its own
// timer through Snapshot. Safe for concurrent use: the subscribe callback runs on paho's own
// goroutines (one per incoming message when --ordered is off), while display reads from
// another.
//
// Received only ever increases. --reset-after used to reset it directly; now that is display
// state (see display.RunSubscribeLog), so this always reports the true total for the run.
type SubscribeProgress struct {
	received      atomic.Int64
	lastMessageAt atomicTime
	subscribedAt  atomicTime

	// done closes once the run is over: ctx was cancelled and the client disconnected.
	done chan struct{}
}

// NewSubscribeProgress creates a progress value for one subscribe run.
func NewSubscribeProgress() *SubscribeProgress {
	return &SubscribeProgress{done: make(chan struct{})}
}

// Done closes once the run is over.
func (progress *SubscribeProgress) Done() <-chan struct{} {
	return progress.done
}

// recordMessage records that one message was received, now.
func (progress *SubscribeProgress) recordMessage() {
	progress.received.Add(1)
	progress.lastMessageAt.set(time.Now())
}

// recordSubscribed records that the broker acknowledged the subscribe request, now.
func (progress *SubscribeProgress) recordSubscribed() {
	progress.subscribedAt.set(time.Now())
}

// finish marks the run as over.
func (progress *SubscribeProgress) finish() {
	close(progress.done)
}

// Snapshot returns a plain-value copy of the current progress.
func (progress *SubscribeProgress) Snapshot() SubscribeSnapshot {
	return SubscribeSnapshot{
		Received:      progress.received.Load(),
		LastMessageAt: progress.lastMessageAt.load(),
		SubscribedAt:  progress.subscribedAt.load(),
	}
}
