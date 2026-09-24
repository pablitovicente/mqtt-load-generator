package cli

import (
	"github.com/spf13/cobra"
)

// newDumpCommand creates the dump subcommand: print each received payload as text.
func newDumpCommand(connection *Connection) *cobra.Command {
	dumpCommand := &cobra.Command{
		Use:   "dump",
		Short: "Dump received MQTT payloads as text",
		Long:  "Subscribe to an MQTT topic and print each received payload as text.",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := connection.Validate(); err != nil {
				return err
			}

			return printConfig(cmd, connection, nil, nil)
		},

		SilenceUsage: true,
	}

	return dumpCommand
}
