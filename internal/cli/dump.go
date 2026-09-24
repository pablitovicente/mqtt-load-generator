package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// newDumpCommand creates the dump subcommand: print each received payload as text.
func newDumpCommand(connection *Connection, connect connectFunc) *cobra.Command {
	dumpCommand := &cobra.Command{
		Use:   "dump",
		Short: "Dump received MQTT payloads as text",
		Long:  "Subscribe to an MQTT topic and print each received payload as text, one per line.",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := connection.Validate(); err != nil {
				return err
			}

			logger := newLogger(connection.LogLevel, cmd.ErrOrStderr())

			client, err := connect(cmd.Context(), connection.toBrokerOptions(), connection.effectiveClientID(), logger)
			if err != nil {
				return fmt.Errorf("connecting to broker: %w", err)
			}

			return mqttload.RunDump(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), cmd.OutOrStdout())
		},

		SilenceUsage: true,
	}

	return dumpCommand
}
