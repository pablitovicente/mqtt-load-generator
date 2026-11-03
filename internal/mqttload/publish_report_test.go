package mqttload

import (
	"testing"
	"time"
)

// TestBuildPublishReport checks each group of the report against hand-made progress and stats.
func TestBuildPublishReport(t *testing.T) {
	progress := NewPublishProgress(2)
	stats := NewPublishStats(2, false)
	options := PublishOptions{Clients: 2, InFlight: 4, QoS: 1, Size: 64}

	startedAt := time.Now().Add(-10 * time.Second)
	progress.publishStartedAt.set(startedAt)
	progress.finishedAt.set(startedAt.Add(10 * time.Second))

	progress.counterFor(0).published.Add(100)
	progress.counterFor(0).acked.Add(100)
	progress.counterFor(1).published.Add(60)
	progress.counterFor(1).acked.Add(50)
	progress.counterFor(1).failed.Add(10)

	stats.clients[0].connectDuration = 2 * time.Millisecond
	stats.clients[1].connectDuration = 4 * time.Millisecond
	stats.clients[0].inflightWait = 5 * time.Second
	stats.clients[1].inflightWait = 15 * time.Second
	stats.clients[0].finishedAt = startedAt.Add(10 * time.Second)
	stats.clients[1].finishedAt = startedAt.Add(5 * time.Second)

	// With nothing recorded: no ack latency, and values is an empty list.
	report := BuildPublishReport(progress, stats, options)

	if report.AckLatencyMicroseconds != nil {
		t.Errorf("ack latency = %+v, want nil", report.AckLatencyMicroseconds)
	}
	if report.AcksPerInterval.Values == nil || len(report.AcksPerInterval.Values) != 0 {
		t.Errorf("values = %#v, want an empty, non-nil list", report.AcksPerInterval.Values)
	}
	if report.AcksPerInterval.Lowest != 0 || report.AcksPerInterval.Highest != 0 {
		t.Errorf("lowest/highest = %d/%d, want 0/0", report.AcksPerInterval.Lowest, report.AcksPerInterval.Highest)
	}

	// With recorded values.
	stats.ackedPerInterval = []int64{20, 80, 50}
	stats.clients[0].sharedLatency.record(1000)
	stats.clients[1].sharedLatency.record(2000)

	report = BuildPublishReport(progress, stats, options)

	if report.Settings != (PublishReportSettings{Clients: 2, InFlight: 4, QoS: 1, PayloadBytes: 64}) {
		t.Errorf("settings = %+v", report.Settings)
	}
	if report.Messages != (PublishReportMessages{Published: 160, Acked: 150, Failed: 10}) {
		t.Errorf("messages = %+v", report.Messages)
	}
	if report.Run.ElapsedSeconds != 10 || !report.Run.StartedAt.Equal(startedAt) || report.Run.GoMaxProcs < 1 {
		t.Errorf("run = %+v", report.Run)
	}

	acks := report.AcksPerInterval
	if acks.IntervalSeconds != 1 || acks.Average != 15 || acks.Lowest != 20 || acks.Highest != 80 || len(acks.Values) != 3 {
		t.Errorf("acksPerInterval = %+v", acks)
	}

	if report.AckLatencyMicroseconds == nil {
		t.Fatal("ack latency is nil, want values")
	}
	if report.AckLatencyMicroseconds.Max < 2000 || report.AckLatencyMicroseconds.P50 < 1000 {
		t.Errorf("ack latency = %+v", report.AckLatencyMicroseconds)
	}

	if report.ConnectLatencyMicroseconds.Max < 4000 || report.ConnectLatencyMicroseconds.P50 < 2000 {
		t.Errorf("connect latency = %+v", report.ConnectLatencyMicroseconds)
	}

	// 20 seconds of waiting out of 2 clients x 10 seconds.
	if report.InflightWait.ShareOfRunTime != 1 {
		t.Errorf("share of run time = %v, want 1 (clamped)", report.InflightWait.ShareOfRunTime)
	}

	// Client 0: 100 acks in 10 s. Client 1: 50 acks in 5 s.
	rates := report.PerClientAckedPerSecond
	if rates.Slowest != 10 || rates.Median != 10 || rates.Fastest != 10 {
		t.Errorf("per-client rates = %+v, want all 10", rates)
	}
}
