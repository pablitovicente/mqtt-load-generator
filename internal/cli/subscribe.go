package cli

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/v2/internal/display"
	"github.com/pablitovicente/mqtt-load-generator/v2/internal/mqttload"
)

// newSubscribeCommand creates the sub subcommand: count received messages.
func newSubscribeCommand(connection *Connection, connect connectFunc) *cobra.Command {
	subscribe := &Subscribe{}

	subscribeCommand := &cobra.Command{
		Use:   "sub",
		Short: "Subscribe to MQTT messages",
		Long:  "Subscribe to an MQTT topic and count received messages.",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := connection.Validate(); err != nil {
				return err
			}

			if err := subscribe.Validate(); err != nil {
				return err
			}

			logger := newLogger(connection.LogLevel, cmd.ErrOrStderr())

			brokerOptions := connection.toBrokerOptions()
			brokerOptions.Ordered = subscribe.Ordered

			client, err := connect(cmd.Context(), brokerOptions, connection.effectiveClientID(), logger)
			if err != nil {
				// broker.Dial's error already says "connecting to <broker URL>".
				return err
			}

			progress := mqttload.NewSubscribeProgress()
			stats := mqttload.NewSubscribeStats(subscribe.Benchmark)
			resetAfter := time.Duration(subscribe.ResetAfter * float64(time.Second))

			// display runs on its own goroutine, reading progress on its own timer. We wait for
			// it to finish its final output before returning, so nothing prints after the
			// command has already returned.
			displayStopped := make(chan struct{})
			go func() {
				defer close(displayStopped)
				if subscribe.DisableBar {
					display.RunSubscribeLog(progress, logger, resetAfter)
				} else {
					display.RunSubscribeBar(progress, logger, cmd.ErrOrStderr())
				}
			}()

			err = mqttload.RunSubscribe(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), progress, stats, subscribe.Count)
			<-displayStopped

			// The report goes to stdout. It is skipped when the subscribe never succeeded.
			if progress.Snapshot().SubscribedAt.IsZero() {
				return err
			}

			report := mqttload.BuildSubscribeReport(progress, stats, connection.Topic, byte(connection.QoS), subscribe.Count)
			reportErr := display.WriteReport(cmd.OutOrStdout(), report)
			if err == nil {
				return reportErr
			}

			return err
		},

		SilenceUsage: true,
	}

	registerSubscribeFlags(subscribeCommand.Flags(), subscribe)

	return subscribeCommand
}

// registerSubscribeFlags adds the sub-only flags to a flag set.
func registerSubscribeFlags(flags *pflag.FlagSet, subscribe *Subscribe) {
	flags.BoolVar(&subscribe.DisableBar, "disable-bar", false, "Print statistics as log lines instead of a progress bar")
	flags.Float64Var(&subscribe.ResetAfter, "reset-after", 30, "Reset counter after N seconds without a message")

	// Off by default: counting doesn't care about order, and unordered is faster.
	flags.BoolVar(&subscribe.Ordered, "ordered", false, orderedHelp)

	flags.Int64Var(&subscribe.Count, "count", 0, "Stop after this many messages and write the report. 0 means no limit")
	flags.BoolVar(&subscribe.Benchmark, "benchmark", false, "Read the timestamp from --benchmark payloads sent by pub --benchmark and measure end-to-end latency. Needs the publisher's and subscriber's clocks in sync")
}
