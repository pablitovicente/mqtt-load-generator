package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/internal/display"
	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// newPublishCommand creates the pub subcommand. It shares its Connection and Publish values with
// the root command, so "mqtt-load-generator pub ..." and "mqtt-load-generator ..." behave the
// same way and run through the same code.
func newPublishCommand(connection *Connection, publish *Publish, connect connectFunc) *cobra.Command {
	publishCommand := &cobra.Command{
		Use:   "pub",
		Short: "Publish MQTT messages",
		Long:  "Publish load to an MQTT broker.",
		Args:  cobra.NoArgs,

		RunE: newPublishRunFunction(connection, publish, connect),

		SilenceUsage: true,
	}

	registerPublishFlags(publishCommand.Flags(), publish)

	return publishCommand
}

// newPublishRunFunction builds the function cobra calls when pub (or the bare command) runs. It
// doesn't publish anything itself; it returns a function that, when called, validates, connects
// every client (in parallel, respecting --connect-concurrency), then runs the publish loop.
func newPublishRunFunction(connection *Connection, publish *Publish, connect connectFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := connection.Validate(); err != nil {
			return err
		}

		if err := publish.ValidateWithConnection(connection); err != nil {
			return err
		}

		logger := newLogger(connection.LogLevel, cmd.ErrOrStderr())
		brokerOptions := connection.toBrokerOptions()

		// mqttload knows nothing about broker.Dial: it asks for one client at a time, numbered
		// 1..N, and we connect each one from the parsed connection flags. Every client gets its
		// own client ID: --clientID (Validate only allows that together with a single client)
		// or a freshly generated one.
		connectClient := func(ctx context.Context, clientNumber int) (mqttload.Publisher, error) {
			return connect(ctx, brokerOptions, connection.effectiveClientID(), logger)
		}

		publishOptions := mqttload.PublishOptions{
			Topic:                connection.Topic,
			QoS:                  byte(connection.QoS),
			Count:                publish.Count,
			Size:                 publish.Size,
			IntervalMilliseconds: publish.Interval,
			Schedule:             publish.Schedule,
			Clients:              publish.Clients,
			Suffix:               publish.Suffix,
			Benchmark:            publish.Benchmark,
			InFlight:             publish.InFlight,
			AckTimeout:           publish.AckTimeout,
			ConnectConcurrency:   publish.ConnectConcurrency,
		}

		progress := mqttload.NewPublishProgress(publish.Clients)
		totalMessages := int64(publish.Clients) * int64(publish.Count)

		// display runs on its own goroutine, reading progress on its own timer. We wait for it
		// to finish its final output before returning, so nothing prints after the command has
		// already returned.
		displayStopped := make(chan struct{})
		go func() {
			defer close(displayStopped)
			display.RunPublishProgress(progress, totalMessages, logger, cmd.ErrOrStderr())
		}()

		// The first Ctrl-C cancels cmd.Context(), but the run keeps going until in-flight
		// publishes finish or --ack-timeout runs out, which can take a while and looks frozen
		// otherwise. Log a line about it once. runFinished closes as soon as RunPublish
		// returns, so this goroutine always exits, whichever of the two happens first.
		runFinished := make(chan struct{})
		go func() {
			select {
			case <-cmd.Context().Done():
				logger.Info(fmt.Sprintf(
					"stopping: waiting up to %s for in-flight publishes, press Ctrl-C again to quit",
					publish.AckTimeout))
			case <-runFinished:
			}
		}()

		err := mqttload.RunPublish(cmd.Context(), connectClient, logger, publishOptions, progress)
		close(runFinished)
		<-displayStopped

		return err
	}
}

// registerPublishFlags adds the publish flags to a flag set. Called for both the root command
// and pub, against the same Publish value.
func registerPublishFlags(flags *pflag.FlagSet, publish *Publish) {
	flags.IntVarP(&publish.Count, "count", "c", 1000, "Number of messages to send")
	flags.IntVarP(&publish.Size, "size", "s", 100, "Size in bytes of the message payload")
	flags.IntVarP(&publish.Interval, "interval", "i", 1, "Milliseconds to wait between messages")
	flags.StringVarP(&publish.Schedule, "schedule", "z", "normal",
		"Distribution of time between messages: 'flat' always waits the interval, "+
			"'normal' waits a random amount with mean equal to the interval and standard "+
			"deviation half the interval, 'random' waits a random amount with mean equal to the interval")
	flags.IntVarP(&publish.Clients, "clients", "n", 1, "Number of concurrent MQTT clients")
	flags.BoolVar(&publish.Suffix, "suffix", false, "Add the client number, from 1 to N, as a sub-topic under --topic")
	flags.BoolVar(&publish.Benchmark, "benchmark", false, "Use a benchmark payload: JSON with a timestamp and padding, for latency measurement")
	flags.IntVar(&publish.InFlight, "inflight", 1, "Maximum number of unacknowledged publishes at once per client (1..65535)")
	flags.DurationVar(&publish.AckTimeout, "ack-timeout", 30*time.Second, "How long to wait for a publish to be acknowledged before it counts as timed out")
	flags.IntVar(&publish.ConnectConcurrency, "connect-concurrency", 16, "Maximum number of clients connecting at the same time. At most --clients are used.")
}
