package mqttload

import (
	"context"
	"math/rand/v2"
	"time"
)

// pacer decides how long to wait between messages, following --schedule, and does the actual
// waiting. Each client gets its own pacer with its own random source, so tests can pass a
// fixed-seed one for repeatable numbers, and production clients don't all draw from the same
// generator.
type pacer struct {
	schedule             string
	intervalMilliseconds int
	random               *rand.Rand
}

// newPacer builds a pacer for one client.
func newPacer(schedule string, intervalMilliseconds int, random *rand.Rand) *pacer {
	return &pacer{schedule: schedule, intervalMilliseconds: intervalMilliseconds, random: random}
}

// next works out how long to wait before the next message.
//
// v1 converted milliseconds to a time.Duration before multiplying by the random factor, which
// truncated to a whole number of milliseconds first and threw away the fraction. That halved the
// real average for "normal" and "random" schedules (an average of 1ms became an average of
// 0.5ms). Multiplying as a float first and converting to a Duration only at the end keeps the
// fraction.
func (pacer *pacer) next() time.Duration {
	if pacer.intervalMilliseconds == 0 {
		return 0
	}

	interval := float64(pacer.intervalMilliseconds)
	milliseconds := interval

	switch pacer.schedule {
	case "normal":
		milliseconds = interval + interval*pacer.random.NormFloat64()/2
		if milliseconds < 0 {
			milliseconds = 0
		}
	case "random":
		milliseconds = interval * 2 * pacer.random.Float64()
	}

	return time.Duration(milliseconds * float64(time.Millisecond))
}

// wait sleeps for next(), or returns early if ctx is cancelled first. It reports whether it
// waited the full duration: false means ctx was cancelled before that.
func (pacer *pacer) wait(ctx context.Context) bool {
	duration := pacer.next()
	if duration <= 0 {
		return true
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
