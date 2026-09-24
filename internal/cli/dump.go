package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// newDumpCommand creates the dump subcommand: print each received payload as text.
func newDumpCommand(connection *Connection, connect connectFunc) *cobra.Command {
	dump := &Dump{}

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

			brokerOptions := connection.toBrokerOptions()
			brokerOptions.Ordered = dump.Ordered

			client, err := connect(cmd.Context(), brokerOptions, connection.effectiveClientID(), logger)
			if err != nil {
				return fmt.Errorf("connecting to broker: %w", err)
			}

			dumpOptions := mqttload.DumpOptions{ShowTopic: dump.ShowTopic}

			return mqttload.RunDump(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), dumpOptions, cmd.OutOrStdout())
		},

		SilenceUsage: true,
	}

	registerDumpFlags(dumpCommand.Flags(), dump)

	return dumpCommand
}

// registerDumpFlags adds the dump-only flags to a flag set.
func registerDumpFlags(flags *pflag.FlagSet, dump *Dump) {
	// On by default: a dump is read by a person, and lines out of order are confusing.
	flags.BoolVar(&dump.Ordered, "ordered", true, orderedHelp)

	flags.BoolVar(&dump.ShowTopic, "show-topic", false, "Print the topic and a tab before each payload")
}
