package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newPublishCommand creates the pub subcommand. It shares its Connection and Publish values with
// the root command, so "mqtt-load-generator pub ..." and "mqtt-load-generator ..." behave the
// same way and are validated and printed by the same function.
func newPublishCommand(connection *Connection, publish *Publish) *cobra.Command {
	publishCommand := &cobra.Command{
		Use:   "pub",
		Short: "Publish MQTT messages",
		Long:  "Publish load to an MQTT broker.",
		Args:  cobra.NoArgs,

		RunE: runPublish(connection, publish),

		SilenceUsage: true,
	}

	registerPublishFlags(publishCommand.Flags(), publish)

	return publishCommand
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
