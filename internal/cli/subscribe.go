package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

			client, err := connect(cmd.Context(), connection.toBrokerOptions(), connection.effectiveClientID(), logger)
			if err != nil {
				return fmt.Errorf("connecting to broker: %w", err)
			}

			subscribeOptions := mqttload.SubscribeOptions{
				DisableBar: subscribe.DisableBar,
				ResetAfter: time.Duration(subscribe.ResetAfter * float64(time.Second)),
			}

			return mqttload.RunSubscribe(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), subscribeOptions, cmd.ErrOrStderr())
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
}
