package cli

import (
	"fmt"
	"time"
)

// Connection holds the settings needed to open an MQTT connection. It is shared by
// every subcommand: pub, sub and dump all connect the same way.
type Connection struct {
	Host             string
	Port             int
	Username         string
	Password         string
	Topic            string
	QoS              int
	CleanSession     bool
	ClientID         string
	KeepAliveTimeout int64
	LogLevel         string
	TLS              TLS
	MQTTS            bool
	Insecure         bool
}

// TLS holds the three files needed for mutual TLS. All three are required together, or none.
type TLS struct {
	CA   string
	Cert string
	Key  string
}

// Publish holds the settings for the pub command (and the bare command, which runs pub).
type Publish struct {
	Count              int
	Size               int
	Interval           int
	Schedule           string
	Clients            int
	Suffix             bool
	Benchmark          bool
	InFlight           int
	AckTimeout         time.Duration
	ConnectConcurrency int
}

// Subscribe holds the settings for the sub command.
type Subscribe struct {
	DisableBar bool
	ResetAfter float64
	Ordered    bool
}

// Dump holds the settings for the dump command.
type Dump struct {
	Ordered   bool
	ShowTopic bool
	JSON      bool
}

// Validate checks the connection settings and returns an error describing the first
// problem found.
func (connection *Connection) Validate() error {
	if connection.QoS < 0 || connection.QoS > 2 {
		return fmt.Errorf("--qos must be 0, 1 or 2 (got %d)", connection.QoS)
	}

	if connection.Port < 1 || connection.Port > 65535 {
		return fmt.Errorf("--port must be 1..65535 (got %d)", connection.Port)
	}

	if connection.KeepAliveTimeout < 0 {
		return fmt.Errorf("--keepAliveTimeout must be at least 0 (got %d)", connection.KeepAliveTimeout)
	}

	if _, err := parseLogLevel(connection.LogLevel); err != nil {
		return fmt.Errorf("--log-level must be debug, info, warn or error (got %q)", connection.LogLevel)
	}

	// --cert, --ca and --key only make sense together: count how many were given and
	// reject one or two of the three, since that silently drops to plain TCP otherwise.
	tlsFieldsSet := 0
	if connection.TLS.CA != "" {
		tlsFieldsSet++
	}
	if connection.TLS.Cert != "" {
		tlsFieldsSet++
	}
	if connection.TLS.Key != "" {
		tlsFieldsSet++
	}
	if tlsFieldsSet == 1 || tlsFieldsSet == 2 {
		return fmt.Errorf("--cert, --ca, and --key must all be set together or none at all")
	}

	// Without TLS there is no certificate to skip checking, so --insecure would do nothing and
	// the connection (password included) would go over plain TCP.
	if connection.Insecure && !connection.MQTTS && tlsFieldsSet == 0 {
		return fmt.Errorf("--insecure only works with --mqtts or --cert/--ca/--key; without them the connection is plain TCP")
	}

	return nil
}

// Validate checks the publish settings on their own, without reference to the connection.
func (publish *Publish) Validate() error {
	if publish.Count < 1 {
		return fmt.Errorf("--count must be at least 1 (got %d)", publish.Count)
	}

	if publish.Size < 0 {
		return fmt.Errorf("--size must be at least 0 (got %d)", publish.Size)
	}

	if publish.Interval < 0 {
		return fmt.Errorf("--interval must be at least 0 (got %d)", publish.Interval)
	}

	if publish.Schedule != "flat" && publish.Schedule != "normal" && publish.Schedule != "random" {
		return fmt.Errorf("--schedule must be flat, normal or random (got %q)", publish.Schedule)
	}

	if publish.Clients < 1 {
		return fmt.Errorf("--clients must be at least 1 (got %d)", publish.Clients)
	}

	if publish.InFlight < 1 || publish.InFlight > 65535 {
		return fmt.Errorf("--inflight must be 1..65535 (got %d)", publish.InFlight)
	}

	if publish.AckTimeout <= 0 {
		return fmt.Errorf("--ack-timeout must be above 0 (got %v)", publish.AckTimeout)
	}

	if publish.ConnectConcurrency < 1 {
		return fmt.Errorf("--connect-concurrency must be at least 1 (got %d)", publish.ConnectConcurrency)
	}

	return nil
}

// ValidateWithConnection checks the publish settings together with the connection settings,
// for the one rule that spans both: a custom client ID only makes sense with a single client.
func (publish *Publish) ValidateWithConnection(connection *Connection) error {
	if err := publish.Validate(); err != nil {
		return err
	}

	if connection.ClientID != "" && publish.Clients != 1 {
		return fmt.Errorf("--clientID can only be used with --clients 1 (broker allows one connection per client ID)")
	}

	return nil
}

// Validate checks the subscribe settings.
func (subscribe *Subscribe) Validate() error {
	if subscribe.ResetAfter <= 0 {
		return fmt.Errorf("--reset-after must be above 0 (got %v)", subscribe.ResetAfter)
	}
	return nil
}
