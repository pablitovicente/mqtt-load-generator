package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

// printedConfig matches the JSON that printConfig writes, so tests can read it back as typed
// values instead of a map[string]any with type assertions.
type printedConfig struct {
	Connection Connection `json:"connection"`
	Publish    *Publish   `json:"publish"`
	Subscribe  *Subscribe `json:"subscribe"`
}

// runCommand builds a fresh root command, runs it with the given args, and returns everything
// written to stdout/stderr together with any error from Execute. A fresh command is needed
// for every call because flag values live on the Connection/Publish/Subscribe values captured
// by NewRootCommand's closures.
func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	rootCommand := NewRootCommand()

	var output bytes.Buffer
	rootCommand.SetOut(&output)
	rootCommand.SetErr(&output)
	rootCommand.SetArgs(args)

	err := rootCommand.Execute()
	return output.String(), err
}

// decodeConfig parses the JSON a command printed into a printedConfig. It fails the test if
// the output is not valid JSON.
func decodeConfig(t *testing.T, output string) printedConfig {
	t.Helper()

	var config printedConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		t.Fatalf("could not decode config JSON: %v\noutput: %s", err, output)
	}

	return config
}
