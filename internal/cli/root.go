package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pablitovicente/mqtt-load-generator/internal/mqttload"
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

		RunE: runPublish(connection, publish),

		SilenceUsage: true,
	}

	// A "help" flag with no shorthand, registered once here. Persistent flags are merged
	// into every subcommand's flag set before cobra decides whether to add its own -h/--help,
	// so this is enough to keep -h free for host everywhere.
	rootCommand.PersistentFlags().Bool("help", false, "Show help")

	registerConnectionFlags(rootCommand.PersistentFlags(), connection)
	registerPublishFlags(rootCommand.Flags(), publish)

	rootCommand.AddCommand(newPubCommand(connection, publish))
	rootCommand.AddCommand(newSubCommand(connection, connect))
	rootCommand.AddCommand(newDumpCommand(connection))

	return rootCommand
}

// newPubCommand creates the pub subcommand. It shares its Connection and Publish values with
// the root command, so "mqtt-load-generator pub ..." and "mqtt-load-generator ..." behave the
// same way and are validated and printed by the same function.
func newPubCommand(connection *Connection, publish *Publish) *cobra.Command {
	pubCommand := &cobra.Command{
		Use:   "pub",
		Short: "Publish MQTT messages",
		Long:  "Publish load to an MQTT broker.",
		Args:  cobra.NoArgs,

		RunE: runPublish(connection, publish),

		SilenceUsage: true,
	}

	registerPublishFlags(pubCommand.Flags(), publish)

	return pubCommand
}

// newSubCommand creates the sub subcommand: count received messages.
func newSubCommand(connection *Connection, connect connectFunc) *cobra.Command {
	subscribe := &Subscribe{}

	subCommand := &cobra.Command{
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

			subOptions := mqttload.SubOptions{
				DisableBar: subscribe.DisableBar,
				ResetAfter: time.Duration(subscribe.ResetAfter * float64(time.Second)),
			}

			return mqttload.RunSub(cmd.Context(), client, logger, connection.Topic, byte(connection.QoS), subOptions, cmd.ErrOrStderr())
		},

		SilenceUsage: true,
	}

	registerSubscribeFlags(subCommand.Flags(), subscribe)

	return subCommand
}

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

// runPublish builds the RunE function shared by the root command and pub: validate, then
// print the config that would be used to run. Both commands share the same connection and
// publish values, but never run in the same invocation, so sharing is only for reuse of code.
func runPublish(connection *Connection, publish *Publish) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := connection.Validate(); err != nil {
			return err
		}

		if err := publish.ValidateWithConnection(connection); err != nil {
			return err
		}

		return printConfig(cmd, connection, publish, nil)
	}
}

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
	flags.IntVar(&publish.ConnectConcurrency, "connect-concurrency", 16, "Maximum number of clients connecting at the same time")
}

// registerSubscribeFlags adds the sub-only flags to a flag set.
func registerSubscribeFlags(flags *pflag.FlagSet, subscribe *Subscribe) {
	flags.BoolVar(&subscribe.DisableBar, "disable-bar", false, "Print statistics as log lines instead of a progress bar")
	flags.Float64Var(&subscribe.ResetAfter, "reset-after", 30, "Reset counter after N seconds without a message")
}

// printConfig prints the configuration a command would run with, as JSON, with the password
// masked. This stands in for the real command implementation until later iterations.
func printConfig(cmd *cobra.Command, connection *Connection, publish *Publish, subscribe *Subscribe) error {
	maskedConnection := *connection
	if maskedConnection.Password != "" {
		maskedConnection.Password = "****"
	}

	config := map[string]any{"connection": maskedConnection}
	if publish != nil {
		config["publish"] = publish
	}
	if subscribe != nil {
		config["subscribe"] = subscribe
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
