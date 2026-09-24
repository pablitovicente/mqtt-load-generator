package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// environmentVariableByFlag lists the flags that fall back to an environment variable when
// not given on the command line: credentials and TLS files, so they don't need to appear in
// shell history or process listings.
var environmentVariableByFlag = map[string]string{
	"username": "MQTT_USERNAME",
	"password": "MQTT_PASSWORD",
	"ca":       "MQTT_CA",
	"cert":     "MQTT_CERT",
	"key":      "MQTT_KEY",
}

// applyEnvironmentFallback sets flag values from environment variables. A flag given on the
// command line always wins over its environment variable.
func applyEnvironmentFallback(cmd *cobra.Command) error {
	for flagName, environmentName := range environmentVariableByFlag {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil || flag.Changed {
			continue
		}

		value, ok := os.LookupEnv(environmentName)
		if !ok {
			continue
		}

		if err := flag.Value.Set(value); err != nil {
			return err
		}
	}

	return nil
}
