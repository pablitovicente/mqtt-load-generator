package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/internal/display"
	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
)

// newDumpCommand creates the dump subcommand: print each received payload as text.
func newDumpCommand(connection *Connection, connect connectFunc) *cobra.Command {
	dump := &Dump{}

	dumpCommand := &cobra.Command{
		Use:   "dump",
		Short: "Print received MQTT payloads",
		Long: "Subscribe to an MQTT topic and print each received payload, one per line. " +
			"Payloads are printed as quoted strings with control characters escaped (Go's " +
			"strconv.QuoteToASCII), so no payload can send commands to your terminal. " +
			"Use --json for JSON payloads.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := connection.Validate(); err != nil {
				return err
			}

			logger := newLogger(connection.LogLevel, cmd.ErrOrStderr())

			brokerOptions := connection.toBrokerOptions()
			brokerOptions.Ordered = dump.Ordered

			client, err := connect(cmd.Context(), brokerOptions, connection.effectiveClientID(), logger)
			if err != nil {
				// broker.Dial's error already says "connecting to <broker URL>".
				return err
			}

			printer := display.NewMessagePrinter(cmd.OutOrStdout(), logger, display.DumpOptions{ShowTopic: dump.ShowTopic, JSON: dump.JSON})

			return mqttload.RunDump(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), printer)
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

	flags.BoolVar(&dump.ShowTopic, "show-topic", false, "Print the topic and a tab before each payload (default format only; --json always includes the topic)")

	flags.BoolVar(&dump.JSON, "json", false,
		`Print each message as one JSON object per line: {"topic":...,"payload":...}. `+
			"Payloads that are not valid JSON are skipped with a warning")
}
