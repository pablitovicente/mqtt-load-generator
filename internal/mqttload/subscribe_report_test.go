package mqttload

import (
	"testing"
	"time"
)

// TestBuildSubscribeReport checks each group of the report against hand-made progress and stats.
func TestBuildSubscribeReport(t *testing.T) {
	progress := NewSubscribeProgress()
	stats := NewSubscribeStats(true)

	startedAt := time.Now().Add(-10 * time.Second)
	progress.subscribedAt.set(startedAt)
	progress.finishedAt.set(startedAt.Add(10 * time.Second))
	progress.received.Add(150)
	progress.receivedBytes.Add(9600)

	stats.withoutTimestamp.Add(3)
	stats.negativeLatency.Add(2)

	// With nothing recorded: no latency, and values is an empty list.
	report := BuildSubscribeReport(progress, stats, "bench/#", 1, 500)

	if report.EndToEndLatencyMilliseconds != nil {
		t.Errorf("latency = %+v, want nil", report.EndToEndLatencyMilliseconds)
	}
	if report.ReceivedPerInterval.Values == nil || len(report.ReceivedPerInterval.Values) != 0 {
		t.Errorf("values = %#v, want an empty, non-nil list", report.ReceivedPerInterval.Values)
	}

	// With recorded values.
	stats.receivedPerInterval = []int64{20, 80, 50}
	stats.latency.forClient(0).record(3)
	stats.latency.forClient(1).record(20)

	report = BuildSubscribeReport(progress, stats, "bench/#", 1, 500)

	if report.Settings != (SubscribeReportSettings{Topic: "bench/#", QoS: 1, Count: 500}) {
		t.Errorf("settings = %+v", report.Settings)
	}
	if report.Messages != (SubscribeReportMessages{Received: 150, Bytes: 9600, WithoutTimestamp: 3, NegativeLatency: 2}) {
		t.Errorf("messages = %+v", report.Messages)
	}
	if report.Run.ElapsedSeconds != 10 || !report.Run.StartedAt.Equal(startedAt) || report.Run.GoMaxProcs < 1 {
		t.Errorf("run = %+v", report.Run)
	}

	interval := report.ReceivedPerInterval
	if interval.IntervalSeconds != 1 || interval.Average != 15 || interval.Lowest != 20 || interval.Highest != 80 || len(interval.Values) != 3 {
		t.Errorf("receivedPerInterval = %+v", interval)
	}

	latency := report.EndToEndLatencyMilliseconds
	if latency == nil || latency.Max != 20 || latency.P50 != 3 {
		t.Errorf("latency = %+v, want p50 3 and max 20", latency)
	}
}
