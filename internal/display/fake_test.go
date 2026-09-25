package display

import (
	"sync"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// fakePublishProgress is a publishProgressSource a test can drive directly: set a snapshot,
// then close connecting or the whole run whenever the test wants, instead of going through a
// real mqttload.PublishProgress and its timing.
type fakePublishProgress struct {
	mutex sync.Mutex

	snapshot        mqttload.PublishSnapshot
	connectFinished chan struct{}
	done            chan struct{}
}

func newFakePublishProgress() *fakePublishProgress {
	return &fakePublishProgress{
		connectFinished: make(chan struct{}),
		done:            make(chan struct{}),
	}
}

func (progress *fakePublishProgress) Snapshot() mqttload.PublishSnapshot {
	progress.mutex.Lock()
	defer progress.mutex.Unlock()
	return progress.snapshot
}

func (progress *fakePublishProgress) set(snapshot mqttload.PublishSnapshot) {
	progress.mutex.Lock()
	progress.snapshot = snapshot
	progress.mutex.Unlock()
}

func (progress *fakePublishProgress) ConnectFinished() <-chan struct{} {
	return progress.connectFinished
}
func (progress *fakePublishProgress) Done() <-chan struct{} { return progress.done }
func (progress *fakePublishProgress) finishConnecting()     { close(progress.connectFinished) }
func (progress *fakePublishProgress) finish()               { close(progress.done) }

// fakeSubscribeProgress is a subscribeProgressSource a test can drive directly.
type fakeSubscribeProgress struct {
	mutex sync.Mutex

	snapshot mqttload.SubscribeSnapshot
	done     chan struct{}
}

func newFakeSubscribeProgress() *fakeSubscribeProgress {
	return &fakeSubscribeProgress{done: make(chan struct{})}
}

func (progress *fakeSubscribeProgress) Snapshot() mqttload.SubscribeSnapshot {
	progress.mutex.Lock()
	defer progress.mutex.Unlock()
	return progress.snapshot
}

func (progress *fakeSubscribeProgress) set(snapshot mqttload.SubscribeSnapshot) {
	progress.mutex.Lock()
	progress.snapshot = snapshot
	progress.mutex.Unlock()
}

func (progress *fakeSubscribeProgress) Done() <-chan struct{} { return progress.done }
func (progress *fakeSubscribeProgress) finish()               { close(progress.done) }
