package mqttload

import (
	"sync/atomic"
	"time"
)

// atomicTime stores a time.Time as UnixNano, so it can be set and read concurrently without a
// lock. The zero value reads back as the zero time.Time (meaning "never set").
type atomicTime struct {
	nano atomic.Int64
}

func (stored *atomicTime) set(value time.Time) {
	stored.nano.Store(value.UnixNano())
}

func (stored *atomicTime) load() time.Time {
	nano := stored.nano.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}
