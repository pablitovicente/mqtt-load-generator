package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// flagSpec names a flag's long name and, where it has one, its shorthand.
type flagSpec struct {
	long  string
	short string
}

var connectionFlagSpecs = []flagSpec{
	{"host", "h"},
	{"port", "p"},
	{"username", "u"},
	{"password", "P"},
	{"topic", "t"},
	{"qos", "q"},
	{"ca", ""},
	{"cert", ""},
	{"key", ""},
	{"insecure", ""},
	{"mqtts", ""},
	{"cleanSession", ""},
	{"clientID", ""},
	{"keepAliveTimeout", ""},
	{"log-level", ""},
	{"help", ""},
}

var publishFlagSpecs = []flagSpec{
	{"count", "c"},
	{"size", "s"},
	{"interval", "i"},
	{"schedule", "z"},
	{"clients", "n"},
	{"suffix", ""},
	{"benchmark", ""},
	{"inflight", ""},
	{"ack-timeout", ""},
	{"connect-concurrency", ""},
}

var subscribeFlagSpecs = []flagSpec{
	{"disable-bar", ""},
	{"reset-after", ""},
}

// TestFlagMapping checks that every flag lands on the commands the plan says it should:
// connection flags everywhere, publish flags on root and pub, subscribe flags only on sub.
func TestFlagMapping(t *testing.T) {
	tests := []struct {
		commandName string
		wantFlags   []flagSpec
	}{
		{"", append(append([]flagSpec{}, connectionFlagSpecs...), publishFlagSpecs...)},
		{"pub", append(append([]flagSpec{}, connectionFlagSpecs...), publishFlagSpecs...)},
		{"sub", append(append([]flagSpec{}, connectionFlagSpecs...), subscribeFlagSpecs...)},
		{"dump", connectionFlagSpecs},
	}

	for _, tt := range tests {
		name := tt.commandName
		if name == "" {
			name = "root"
		}

		t.Run(name, func(t *testing.T) {
			rootCommand := NewRootCommand()

			command := rootCommand
			if tt.commandName != "" {
				for _, sub := range rootCommand.Commands() {
					if sub.Name() == tt.commandName {
						command = sub
						break
					}
				}
			}

			// Merge in flags inherited from root, the way cobra does right before parsing.
			command.InitDefaultHelpFlag()

			for _, flag := range tt.wantFlags {
				if command.Flags().Lookup(flag.long) == nil {
					t.Errorf("expected --%s on %q, not found", flag.long, name)
				}

				if flag.short != "" && command.Flags().ShorthandLookup(flag.short) == nil {
					t.Errorf("expected -%s on %q, not found", flag.short, name)
				}
			}
		})
	}
}

// TestDefaults checks the default configuration printed by root (bare command), pub, sub and
// dump, and confirms the bare command behaves the same as pub.
func TestDefaults(t *testing.T) {
	wantConnection := Connection{
		Host:             "localhost",
		Port:             1883,
		Topic:            "/load",
		QoS:              1,
		CleanSession:     true,
		KeepAliveTimeout: 5,
		LogLevel:         "info",
	}

	wantPublish := Publish{
		Count:              1000,
		Size:               100,
		Interval:           1,
		Schedule:           "normal",
		Clients:            1,
		InFlight:           1,
		AckTimeout:         30 * time.Second,
		ConnectConcurrency: 16,
	}

	wantSubscribe := Subscribe{ResetAfter: 30}

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, config printedConfig)
	}{
		{"bare command runs pub", nil, func(t *testing.T, config printedConfig) {
			if config.Connection != wantConnection {
				t.Errorf("connection = %+v, want %+v", config.Connection, wantConnection)
			}
			if config.Publish == nil || *config.Publish != wantPublish {
				t.Errorf("publish = %+v, want %+v", config.Publish, wantPublish)
			}
			if config.Subscribe != nil {
				t.Errorf("expected no subscribe config, got %+v", config.Subscribe)
			}
		}},
		{"pub", []string{"pub"}, func(t *testing.T, config printedConfig) {
			if config.Connection != wantConnection {
				t.Errorf("connection = %+v, want %+v", config.Connection, wantConnection)
			}
			if config.Publish == nil || *config.Publish != wantPublish {
				t.Errorf("publish = %+v, want %+v", config.Publish, wantPublish)
			}
		}},
		{"sub", []string{"sub"}, func(t *testing.T, config printedConfig) {
			if config.Connection != wantConnection {
				t.Errorf("connection = %+v, want %+v", config.Connection, wantConnection)
			}
			if config.Subscribe == nil || *config.Subscribe != wantSubscribe {
				t.Errorf("subscribe = %+v, want %+v", config.Subscribe, wantSubscribe)
			}
			if config.Publish != nil {
				t.Errorf("expected no publish config, got %+v", config.Publish)
			}
		}},
		{"dump", []string{"dump"}, func(t *testing.T, config printedConfig) {
			if config.Connection != wantConnection {
				t.Errorf("connection = %+v, want %+v", config.Connection, wantConnection)
			}
			if config.Publish != nil {
				t.Errorf("expected no publish config, got %+v", config.Publish)
			}
			if config.Subscribe != nil {
				t.Errorf("expected no subscribe config, got %+v", config.Subscribe)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			tt.check(t, decodeConfig(t, output))
		})
	}
}

// TestHostShortFlagEverywhere checks that -h sets host, not help, on every command.
func TestHostShortFlagEverywhere(t *testing.T) {
	tests := [][]string{
		{"-h", "broker"},
		{"pub", "-h", "broker"},
		{"sub", "-h", "broker"},
		{"dump", "-h", "broker"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := runCommand(t, args...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			config := decodeConfig(t, output)
			if config.Connection.Host != "broker" {
				t.Errorf("expected host broker, got %q", config.Connection.Host)
			}
		})
	}
}

// TestHelpFlagPrintsHelpNotConfig checks that --help on every command prints help text and
// exits without error, and never prints the JSON config.
func TestHelpFlagPrintsHelpNotConfig(t *testing.T) {
	tests := [][]string{
		{"--help"},
		{"pub", "--help"},
		{"sub", "--help"},
		{"dump", "--help"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := runCommand(t, args...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if !strings.Contains(output, "Usage:") {
				t.Errorf("expected help text, got: %s", output)
			}

			var config map[string]any
			if json.Unmarshal([]byte(output), &config) == nil {
				t.Errorf("expected non-JSON help output, got JSON: %s", output)
			}
		})
	}
}

// TestHelpSubcommand checks that cobra's own "help" subcommand still works for a specific
// subcommand.
func TestHelpSubcommand(t *testing.T) {
	output, err := runCommand(t, "help", "sub")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(output, "Subscribe to an MQTT topic") {
		t.Errorf("expected sub's help text, got: %s", output)
	}
}

// TestEnvironmentFallback checks that each of the five env-backed flags is picked up from its
// environment variable when not given on the command line. The TLS variables need the other
// two TLS flags set too, or the partial-TLS validation rejects the run before we can see the
// fallback take effect.
func TestEnvironmentFallback(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		envValue  string
		extraArgs []string
		extract   func(config printedConfig) string
		want      string
	}{
		{
			name:     "username",
			envVar:   "MQTT_USERNAME",
			envValue: "envuser",
			extract:  func(config printedConfig) string { return config.Connection.Username },
			want:     "envuser",
		},
		{
			name:     "password",
			envVar:   "MQTT_PASSWORD",
			envValue: "envpass",
			extract:  func(config printedConfig) string { return config.Connection.Password },
			want:     "****",
		},
		{
			name:      "ca",
			envVar:    "MQTT_CA",
			envValue:  "ca.pem",
			extraArgs: []string{"--cert", "cert.pem", "--key", "key.pem"},
			extract:   func(config printedConfig) string { return config.Connection.TLS.CA },
			want:      "ca.pem",
		},
		{
			name:      "cert",
			envVar:    "MQTT_CERT",
			envValue:  "cert.pem",
			extraArgs: []string{"--ca", "ca.pem", "--key", "key.pem"},
			extract:   func(config printedConfig) string { return config.Connection.TLS.Cert },
			want:      "cert.pem",
		},
		{
			name:      "key",
			envVar:    "MQTT_KEY",
			envValue:  "key.pem",
			extraArgs: []string{"--ca", "ca.pem", "--cert", "cert.pem"},
			extract:   func(config printedConfig) string { return config.Connection.TLS.Key },
			want:      "key.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			output, err := runCommand(t, tt.extraArgs...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			config := decodeConfig(t, output)
			if got := tt.extract(config); got != tt.want {
				t.Errorf("expected %s=%q, got %q", tt.name, tt.want, got)
			}
		})
	}
}

// TestFlagBeatsEnvironment checks that a flag given on the command line wins over its
// environment variable, for each of the five env-backed flags.
func TestFlagBeatsEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
		args     []string
		extract  func(config printedConfig) string
		want     string
	}{
		{
			name:     "username",
			envVar:   "MQTT_USERNAME",
			envValue: "envuser",
			args:     []string{"--username", "flaguser"},
			extract:  func(config printedConfig) string { return config.Connection.Username },
			want:     "flaguser",
		},
		{
			name:     "password",
			envVar:   "MQTT_PASSWORD",
			envValue: "envpass",
			args:     []string{"--password", "flagpass"},
			extract:  func(config printedConfig) string { return config.Connection.Password },
			want:     "****",
		},
		{
			name:     "ca",
			envVar:   "MQTT_CA",
			envValue: "envca.pem",
			args:     []string{"--ca", "flagca.pem", "--cert", "cert.pem", "--key", "key.pem"},
			extract:  func(config printedConfig) string { return config.Connection.TLS.CA },
			want:     "flagca.pem",
		},
		{
			name:     "cert",
			envVar:   "MQTT_CERT",
			envValue: "envcert.pem",
			args:     []string{"--ca", "ca.pem", "--cert", "flagcert.pem", "--key", "key.pem"},
			extract:  func(config printedConfig) string { return config.Connection.TLS.Cert },
			want:     "flagcert.pem",
		},
		{
			name:     "key",
			envVar:   "MQTT_KEY",
			envValue: "envkey.pem",
			args:     []string{"--ca", "ca.pem", "--cert", "cert.pem", "--key", "flagkey.pem"},
			extract:  func(config printedConfig) string { return config.Connection.TLS.Key },
			want:     "flagkey.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			output, err := runCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			config := decodeConfig(t, output)
			if got := tt.extract(config); got != tt.want {
				t.Errorf("expected %s=%q, got %q", tt.name, tt.want, got)
			}
		})
	}
}

// TestFlagsRejectedOnWrongCommand checks that publish-only flags don't work on sub or dump,
// and that a subscribe-only flag doesn't work on pub.
func TestFlagsRejectedOnWrongCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"count rejected on sub", []string{"sub", "--count", "5"}},
		{"count shorthand rejected on sub", []string{"sub", "-c", "5"}},
		{"count shorthand rejected on dump", []string{"dump", "-c", "5"}},
		{"count rejected on dump", []string{"dump", "--count", "5"}},
		{"disable-bar rejected on pub", []string{"pub", "--disable-bar"}},
		{"reset-after rejected on pub", []string{"pub", "--reset-after", "5"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCommand(t, tt.args...)
			if err == nil {
				t.Errorf("expected error, got none")
			}
		})
	}
}

// TestUnknownSubcommandAndStrayArguments checks that an unrecognized subcommand and a stray
// positional argument both fail, on every command.
func TestUnknownSubcommandAndStrayArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown subcommand", []string{"sbu"}},
		{"stray argument on root", []string{"stray"}},
		{"stray argument on pub", []string{"pub", "stray"}},
		{"stray argument on sub", []string{"sub", "stray"}},
		{"stray argument on dump", []string{"dump", "stray"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCommand(t, tt.args...)
			if err == nil {
				t.Errorf("expected error, got none")
			}
		})
	}
}

// TestValidationEndToEnd confirms validation actually runs when going through the CLI, not
// just when calling Validate() directly. The exhaustive per-rule cases live in config_test.go.
func TestValidationEndToEnd(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"bad qos", []string{"-q", "5"}},
		{"bad schedule", []string{"-z", "badschedule"}},
		{"bad inflight", []string{"--inflight", "0"}},
		{"bad log level", []string{"--log-level", "verbose"}},
		{"clientID with multiple clients", []string{"--clientID", "test", "-n", "5"}},
		{"partial tls", []string{"--cert", "cert.pem", "--key", "key.pem"}},
		{"bad reset-after on sub", []string{"sub", "--reset-after", "0"}},
		{"bad port", []string{"-p", "0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCommand(t, tt.args...)
			if err == nil {
				t.Errorf("expected validation error, got none")
			}
		})
	}
}

// TestPasswordMasking checks that the password is masked when set and left as an empty
// string when not.
func TestPasswordMasking(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no password", nil, ""},
		{"password set", []string{"--password", "secret"}, "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			config := decodeConfig(t, output)
			if config.Connection.Password != tt.want {
				t.Errorf("expected password %q, got %q", tt.want, config.Connection.Password)
			}
		})
	}
}
