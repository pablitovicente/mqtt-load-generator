package cli

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/internal/display"
	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
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

			err = mqttload.RunSubscribe(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), progress)
			<-displayStopped

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
}
