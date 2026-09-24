package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewRootCommand builds the command tree: the root command (which runs pub when called with
// no subcommand), plus pub, sub and dump. Connection settings live in one Connection value
// owned here and shared by every subcommand, so the flags that describe how to connect are
// only registered once, on the root command's persistent flags.
//
// Commands connect with broker.Dial. Tests use newRootCommand with a fake connect function
// so they don't need a broker of their own.
func NewRootCommand() *cobra.Command {
	return newRootCommand(connectToBroker)
}

func newRootCommand(connect connectFunc) *cobra.Command {
	connection := &Connection{}
	publish := &Publish{}

	rootCommand := &cobra.Command{
		Use:   "mqtt-load-generator",
		Short: "MQTT load generator and subscriber",
		Long:  "A tool to generate MQTT load, subscribe to messages, or dump received payloads.",
		Args:  cobra.NoArgs,

		// The environment fallback runs once here. Subcommands must not define their own
		// PersistentPreRunE: cobra only runs the nearest one it finds walking up from the
		// command that was actually invoked, so a second one on a subcommand would replace
		// this, not add to it.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return applyEnvironmentFallback(cmd)
		},

		RunE: newPublishRunFunction(connection, publish, connect),

		SilenceUsage: true,
	}

	// A "help" flag with no shorthand, registered once here. Persistent flags are merged
	// into every subcommand's flag set before cobra decides whether to add its own -h/--help,
	// so this is enough to keep -h free for host everywhere.
	rootCommand.PersistentFlags().Bool("help", false, "Show help")

	registerConnectionFlags(rootCommand.PersistentFlags(), connection)
	registerPublishFlags(rootCommand.Flags(), publish)

	rootCommand.AddCommand(newPublishCommand(connection, publish, connect))
	rootCommand.AddCommand(newSubscribeCommand(connection, connect))
	rootCommand.AddCommand(newDumpCommand(connection, connect))

	return rootCommand
}

// orderedHelp is the help text for --ordered on sub and dump. It spells out the trade-off so
// users can choose: ordered keeps the order but is slower.
const orderedHelp = "Handle received messages one at a time, in the order they arrive. " +
	"Slower: each message waits until the one before it has been handled, so under heavy " +
	"load the tool can fall behind the broker. When off, messages are handled in parallel: " +
	"faster, but the order can change"

// registerConnectionFlags adds every connection flag to a flag set. It is called once, on
// the root command's persistent flags, so it must never be called a second time for a
// different Connection value.
func registerConnectionFlags(flags *pflag.FlagSet, connection *Connection) {
	flags.StringVarP(&connection.Host, "host", "h", "localhost", "MQTT host")
	flags.IntVarP(&connection.Port, "port", "p", 1883, "MQTT port")
	flags.StringVarP(&connection.Username, "username", "u", "", "MQTT username (env MQTT_USERNAME)")
	flags.StringVarP(&connection.Password, "password", "P", "", "MQTT password (env MQTT_PASSWORD)")
	flags.StringVarP(&connection.Topic, "topic", "t", "/load", "MQTT topic to publish or subscribe to")
	flags.IntVarP(&connection.QoS, "qos", "q", 1, "MQTT QoS level used by all clients (0, 1 or 2)")
	flags.StringVar(&connection.TLS.CA, "ca", "", "Path to TLS CA file (env MQTT_CA)")
	flags.StringVar(&connection.TLS.Cert, "cert", "", "Path to TLS certificate file (env MQTT_CERT)")
	flags.StringVar(&connection.TLS.Key, "key", "", "Path to TLS private key file (env MQTT_KEY)")
	flags.BoolVar(&connection.Insecure, "insecure", false, "Allow self-signed certificates")
	flags.BoolVar(&connection.MQTTS, "mqtts", false, "Use MQTTS (TLS)")
	flags.BoolVar(&connection.CleanSession, "cleanSession", true, "Use a clean MQTT session instead of resuming a previous one")
	flags.StringVar(&connection.ClientID, "clientID", "", "Custom MQTT client ID (only allowed with --clients 1)")
	flags.Int64Var(&connection.KeepAliveTimeout, "keepAliveTimeout", 5, "Seconds to wait before sending a PING request to the broker")
	flags.StringVar(&connection.LogLevel, "log-level", "info", "Log level (debug, info, warn or error)")
}
