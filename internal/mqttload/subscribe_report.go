package mqttload

import (
	"os"
	"runtime"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// SubscribeReport is the end-of-run report, written as one JSON line. Field names are part of
// the output format.
type SubscribeReport struct {
	Run                         SubscribeReportRun                 `json:"run"`
	Settings                    SubscribeReportSettings            `json:"settings"`
	Messages                    SubscribeReportMessages            `json:"messages"`
	ReceivedPerInterval         SubscribeReportReceivedPerInterval `json:"receivedPerInterval"`
	EndToEndLatencyMilliseconds *SubscribeReportLatency            `json:"endToEndLatencyMilliseconds,omitempty"`
}

type SubscribeReportRun struct {
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	ElapsedSeconds float64   `json:"elapsedSeconds"`
	Host           string    `json:"host"`
	GoMaxProcs     int       `json:"gomaxprocs"`
}

type SubscribeReportSettings struct {
	Topic string `json:"topic"`
	QoS   byte   `json:"qos"`
	Count int64  `json:"count"`
}

type SubscribeReportMessages struct {
	Received         int64 `json:"received"`
	Bytes            int64 `json:"bytes"`
	WithoutTimestamp int64 `json:"withoutTimestamp"`
	NegativeLatency  int64 `json:"negativeLatency"`
}

// SubscribeReportReceivedPerInterval has the same fields as PublishReportAcksPerInterval, so
// both are built by the same function.
type SubscribeReportReceivedPerInterval struct {
	IntervalSeconds float64 `json:"intervalSeconds"`
	Average         float64 `json:"average"`
	Lowest          int64   `json:"lowest"`
	Highest         int64   `json:"highest"`
	Values          []int64 `json:"values"`
}

type SubscribeReportLatency struct {
	P50  int64 `json:"p50"`
	P90  int64 `json:"p90"`
	P99  int64 `json:"p99"`
	P999 int64 `json:"p999"`
	Max  int64 `json:"max"`
}

// BuildSubscribeReport builds the end-of-run report. Call it only after RunSubscribe has
// returned, and only when the subscribe succeeded (SubscribedAt in the snapshot is not zero).
func BuildSubscribeReport(progress *SubscribeProgress, stats *SubscribeStats, topic string, qos byte, count int64) SubscribeReport {
	snapshot := progress.Snapshot()
	elapsedSeconds := snapshot.FinishedAt.Sub(snapshot.SubscribedAt).Seconds()

	// os.Hostname returns an empty name on error, which is fine for the report.
	hostName, _ := os.Hostname()

	return SubscribeReport{
		Run: SubscribeReportRun{
			StartedAt:      snapshot.SubscribedAt,
			FinishedAt:     snapshot.FinishedAt,
			ElapsedSeconds: elapsedSeconds,
			Host:           hostName,
			GoMaxProcs:     runtime.GOMAXPROCS(0),
		},
		Settings: SubscribeReportSettings{
			Topic: topic,
			QoS:   qos,
			Count: count,
		},
		Messages: SubscribeReportMessages{
			Received:         snapshot.Received,
			Bytes:            snapshot.ReceivedBytes,
			WithoutTimestamp: stats.withoutTimestamp.Load(),
			NegativeLatency:  stats.negativeLatency.Load(),
		},
		ReceivedPerInterval:         SubscribeReportReceivedPerInterval(buildAcksPerInterval(stats.receivedPerInterval, snapshot.Received, elapsedSeconds)),
		EndToEndLatencyMilliseconds: buildEndToEndLatency(stats),
	}
}

// buildEndToEndLatency returns nil when no latency was recorded, which is the case without
// --benchmark. The recorded values are milliseconds.
func buildEndToEndLatency(stats *SubscribeStats) *SubscribeReportLatency {
	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)
	stats.latency.addTo(histogram)

	if histogram.TotalCount() == 0 {
		return nil
	}

	return &SubscribeReportLatency{
		P50:  histogram.ValueAtQuantile(50),
		P90:  histogram.ValueAtQuantile(90),
		P99:  histogram.ValueAtQuantile(99),
		P999: histogram.ValueAtQuantile(99.9),
		Max:  histogram.Max(),
	}
}
