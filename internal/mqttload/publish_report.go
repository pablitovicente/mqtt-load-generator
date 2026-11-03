package mqttload

import (
	"math"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// PublishReport is the end-of-run report, written as one JSON line. Field names are part of
// the output format.
type PublishReport struct {
	Run                        PublishReportRun                 `json:"run"`
	Settings                   PublishReportSettings            `json:"settings"`
	Messages                   PublishReportMessages            `json:"messages"`
	AcksPerInterval            PublishReportAcksPerInterval     `json:"acksPerInterval"`
	AckLatencyMicroseconds     *PublishReportAckLatency         `json:"ackLatencyMicroseconds,omitempty"`
	ConnectLatencyMicroseconds PublishReportConnectLatency      `json:"connectLatencyMicroseconds"`
	InflightWait               PublishReportInflightWait        `json:"inflightWait"`
	PerClientAckedPerSecond    PublishReportPerClientAckedRates `json:"perClientAckedPerSecond"`
	Clients                    []PublishReportClient            `json:"clients,omitempty"`
}

type PublishReportRun struct {
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	ElapsedSeconds float64   `json:"elapsedSeconds"`
	Host           string    `json:"host"`
	GoMaxProcs     int       `json:"gomaxprocs"`
}

type PublishReportSettings struct {
	Clients      int  `json:"clients"`
	InFlight     int  `json:"inflight"`
	QoS          byte `json:"qos"`
	PayloadBytes int  `json:"payloadBytes"`
}

type PublishReportMessages struct {
	Published int64 `json:"published"`
	Acked     int64 `json:"acked"`
	Failed    int64 `json:"failed"`
	TimedOut  int64 `json:"timedOut"`
}

type PublishReportAcksPerInterval struct {
	IntervalSeconds float64 `json:"intervalSeconds"`
	Average         float64 `json:"average"`
	Lowest          int64   `json:"lowest"`
	Highest         int64   `json:"highest"`
	Values          []int64 `json:"values"`
}

type PublishReportAckLatency struct {
	P50  int64 `json:"p50"`
	P90  int64 `json:"p90"`
	P99  int64 `json:"p99"`
	P999 int64 `json:"p999"`
	Max  int64 `json:"max"`
}

type PublishReportConnectLatency struct {
	P50 int64 `json:"p50"`
	P99 int64 `json:"p99"`
	Max int64 `json:"max"`
}

type PublishReportInflightWait struct {
	ShareOfRunTime float64 `json:"shareOfRunTime"`
}

type PublishReportPerClientAckedRates struct {
	Slowest float64 `json:"slowest"`
	Median  float64 `json:"median"`
	Fastest float64 `json:"fastest"`
}

// BuildPublishReport builds the end-of-run report. Call it only after RunPublish has returned.
func BuildPublishReport(progress *PublishProgress, stats *PublishStats, options PublishOptions) PublishReport {
	snapshot := progress.Snapshot()

	// Same calculation as the "pub stopped" log line, so both show the same elapsed time.
	elapsed := snapshot.FinishedAt.Sub(snapshot.PublishStartedAt)
	elapsedSeconds := elapsed.Seconds()

	// os.Hostname returns an empty name on error, which is fine for the report.
	hostName, _ := os.Hostname()

	return PublishReport{
		Run: PublishReportRun{
			StartedAt:      snapshot.PublishStartedAt,
			FinishedAt:     snapshot.FinishedAt,
			ElapsedSeconds: elapsedSeconds,
			Host:           hostName,
			GoMaxProcs:     runtime.GOMAXPROCS(0),
		},
		Settings: PublishReportSettings{
			Clients:      options.Clients,
			InFlight:     options.InFlight,
			QoS:          options.QoS,
			PayloadBytes: options.Size,
		},
		Messages: PublishReportMessages{
			Published: snapshot.Published,
			Acked:     snapshot.Acked,
			Failed:    snapshot.Failed,
			TimedOut:  snapshot.TimedOut,
		},
		AcksPerInterval:            buildAcksPerInterval(stats.ackedPerInterval, snapshot.Acked, elapsedSeconds),
		AckLatencyMicroseconds:     buildAckLatency(stats),
		ConnectLatencyMicroseconds: buildConnectLatency(stats),
		InflightWait:               buildInflightWait(stats, elapsed),
		PerClientAckedPerSecond:    buildPerClientRates(progress, stats, snapshot.PublishStartedAt),
		Clients:                    buildClients(progress, stats),
	}
}

func buildAcksPerInterval(values []int64, acked int64, elapsedSeconds float64) PublishReportAcksPerInterval {
	if values == nil {
		values = []int64{}
	}

	result := PublishReportAcksPerInterval{
		IntervalSeconds: ackSampleInterval.Seconds(),
		Values:          values,
	}

	if len(values) > 0 {
		result.Lowest = slices.Min(values)
		result.Highest = slices.Max(values)
	}

	if elapsedSeconds > 0 {
		result.Average = float64(acked) / elapsedSeconds
	}

	return result
}

// buildAckLatency returns nil when no latency was recorded, which is the case at QoS 0.
func buildAckLatency(stats *PublishStats) *PublishReportAckLatency {
	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)
	stats.latency.addTo(histogram)

	if histogram.TotalCount() == 0 {
		return nil
	}

	return &PublishReportAckLatency{
		P50:  histogram.ValueAtQuantile(50),
		P90:  histogram.ValueAtQuantile(90),
		P99:  histogram.ValueAtQuantile(99),
		P999: histogram.ValueAtQuantile(99.9),
		Max:  histogram.Max(),
	}
}

func buildConnectLatency(stats *PublishStats) PublishReportConnectLatency {
	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)

	for _, client := range stats.clients {
		_ = histogram.RecordValue(client.connectDuration.Microseconds())
	}

	return PublishReportConnectLatency{
		P50: histogram.ValueAtQuantile(50),
		P99: histogram.ValueAtQuantile(99),
		Max: histogram.Max(),
	}
}

func buildInflightWait(stats *PublishStats, elapsed time.Duration) PublishReportInflightWait {
	if elapsed <= 0 || len(stats.clients) == 0 {
		return PublishReportInflightWait{}
	}

	var totalWait time.Duration
	for _, client := range stats.clients {
		totalWait += client.inflightWait
	}

	share := float64(totalWait) / (float64(len(stats.clients)) * float64(elapsed))

	return PublishReportInflightWait{ShareOfRunTime: min(max(share, 0), 1)}
}

func buildPerClientRates(progress *PublishProgress, stats *PublishStats, startedAt time.Time) PublishReportPerClientAckedRates {
	if len(stats.clients) == 0 {
		return PublishReportPerClientAckedRates{}
	}

	clientRates := make([]float64, len(stats.clients))
	for i, client := range stats.clients {
		clientSeconds := client.finishedAt.Sub(startedAt).Seconds()
		if clientSeconds > 0 {
			clientRates[i] = float64(progress.counterFor(i).acked.Load()) / clientSeconds
		}
	}

	slices.Sort(clientRates)

	// With an even number of clients, the median is the average of the two middle rates.
	middle := len(clientRates) / 2
	median := clientRates[middle]
	if len(clientRates)%2 == 0 {
		median = (clientRates[middle-1] + clientRates[middle]) / 2
	}

	return PublishReportPerClientAckedRates{
		Slowest: clientRates[0],
		Median:  median,
		Fastest: clientRates[len(clientRates)-1],
	}
}

// PublishReportClient is one client's entry in the report, present only with per-client stats.
type PublishReportClient struct {
	ClientID               string                      `json:"clientId"`
	Acked                  int64                       `json:"acked"`
	AckLatencyMicroseconds *PublishReportClientLatency `json:"ackLatencyMicroseconds,omitempty"`
}

type PublishReportClientLatency struct {
	Mean int64 `json:"mean"`
	P50  int64 `json:"p50"`
	P99  int64 `json:"p99"`
	Max  int64 `json:"max"`
}

// buildClients returns nil when per-client stats were not asked for.
func buildClients(progress *PublishProgress, stats *PublishStats) []PublishReportClient {
	if len(stats.clients) == 0 || stats.clients[0].perClientLatency == nil {
		return nil
	}

	clients := make([]PublishReportClient, len(stats.clients))
	for i, client := range stats.clients {
		clients[i] = PublishReportClient{
			ClientID:               client.clientID,
			Acked:                  progress.counterFor(i).acked.Load(),
			AckLatencyMicroseconds: buildClientAckLatency(client),
		}
	}

	return clients
}

// buildClientAckLatency returns nil when the client recorded no latency, which is the case at QoS 0.
func buildClientAckLatency(client *clientStats) *PublishReportClientLatency {
	histogram := hdrhistogram.New(1, latencyHighestMicroseconds, 2)
	client.perClientLatency.addTo(histogram)

	if histogram.TotalCount() == 0 {
		return nil
	}

	return &PublishReportClientLatency{
		Mean: int64(math.Round(histogram.Mean())),
		P50:  histogram.ValueAtQuantile(50),
		P99:  histogram.ValueAtQuantile(99),
		Max:  histogram.Max(),
	}
}
