package mqttload

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// fixedRandom returns a *rand.Rand seeded the same way every time, so distribution tests get
// the same sequence of numbers on every run.
func fixedRandom() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}

func TestPacer_FlatIsExact(t *testing.T) {
	pacer := newPacer("flat", 10, fixedRandom())

	for i := 0; i < 20; i++ {
		got := pacer.next()
		want := 10 * time.Millisecond
		if got != want {
			t.Fatalf("next() = %v, want %v", got, want)
		}
	}
}

func TestPacer_IntervalZeroNeverWaits(t *testing.T) {
	for _, schedule := range []string{"flat", "normal", "random"} {
		t.Run(schedule, func(t *testing.T) {
			pacer := newPacer(schedule, 0, fixedRandom())

			if got := pacer.next(); got != 0 {
				t.Errorf("next() = %v, want 0", got)
			}

			// wait must return immediately (true) even with an already-cancelled ctx: there is
			// nothing to wait for.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			if !pacer.wait(ctx) {
				t.Errorf("wait() = false, want true (interval 0 never waits, ctx state included)")
			}
		})
	}
}

func TestPacer_NormalNeverNegative(t *testing.T) {
	pacer := newPacer("normal", 1, fixedRandom())

	for i := 0; i < 100000; i++ {
		if got := pacer.next(); got < 0 {
			t.Fatalf("next() = %v, want >= 0", got)
		}
	}
}

// TestPacer_AveragesCloseToInterval checks that "normal" and "random" schedules average out to
// the requested interval over many samples, at a fixed seed. This is the proof that the
// float-then-Duration conversion keeps fractions of a millisecond: v1 converted to a Duration
// before multiplying by the random factor, which truncated to a whole millisecond first and
// silently halved the real average for an interval like 1ms.
func TestPacer_AveragesCloseToInterval(t *testing.T) {
	tests := []struct {
		schedule             string
		intervalMilliseconds int
	}{
		{"normal", 1},
		{"normal", 10},
		{"random", 1},
		{"random", 10},
	}

	const samples = 200000

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			pacer := newPacer(tt.schedule, tt.intervalMilliseconds, fixedRandom())

			var total time.Duration
			for i := 0; i < samples; i++ {
				total += pacer.next()
			}

			averageMilliseconds := float64(total) / float64(samples) / float64(time.Millisecond)
			want := float64(tt.intervalMilliseconds)

			// A generous tolerance: this is a statistical check, not an exact one, but a
			// truncation-to-milliseconds bug (the one being fixed) would show up as an average
			// roughly half of what's expected, which is far outside this window.
			if math.Abs(averageMilliseconds-want) > 0.05*want+0.05 {
				t.Errorf("average = %.4fms, want close to %.4fms", averageMilliseconds, want)
			}
		})
	}
}

func TestPacer_Wait_CtxCancelInterrupts(t *testing.T) {
	// A pacer with a long interval: if wait() actually slept the full duration instead of
	// noticing ctx, this test would time out instead of failing fast.
	pacer := newPacer("flat", 60000, fixedRandom())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)
	go func() {
		done <- pacer.wait(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case waitedFullDuration := <-done:
		if waitedFullDuration {
			t.Errorf("wait() = true, want false (ctx was cancelled)")
		}
	case <-time.After(time.Second):
		t.Fatal("wait() did not return after ctx was cancelled")
	}
}

func TestPacer_Wait_CompletesWhenNotCancelled(t *testing.T) {
	pacer := newPacer("flat", 1, fixedRandom())

	if !pacer.wait(context.Background()) {
		t.Errorf("wait() = false, want true")
	}
}
