package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pablitovicente/mqtt-load-generator/internal/broker"
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
	{"ordered", ""},
}

var dumpFlagSpecs = []flagSpec{
	{"ordered", ""},
	{"show-topic", ""},
	{"json", ""},
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
		{"dump", append(append([]flagSpec{}, connectionFlagSpecs...), dumpFlagSpecs...)},
	}

	for _, tt := range tests {
		name := tt.commandName
		if name == "" {
			name = "root"
		}

		t.Run(name, func(t *testing.T) {
			rootCommand := newRootCommand((&fakeConnector{}).connect)

			command := rootCommand
			if tt.commandName != "" {
				for _, childCommand := range rootCommand.Commands() {
					if childCommand.Name() == tt.commandName {
						command = childCommand
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

// TestDefaultFlagValues checks the default value of every connection and publish flag straight
// from pflag's own registration. Now that pub runs for real, running a default-sized load
// (1000 messages, 1ms apart) just to read its config back would make the suite slow for no
// reason, so this checks the flags directly instead.
func TestDefaultFlagValues(t *testing.T) {
	rootCommand := newRootCommand((&fakeConnector{}).connect)

	tests := []struct {
		flag       string
		want       string
		persistent bool
	}{
		{"host", "localhost", true},
		{"port", "1883", true},
		{"topic", "/load", true},
		{"qos", "1", true},
		{"cleanSession", "true", true},
		{"keepAliveTimeout", "5", true},
		{"log-level", "info", true},
		{"count", "1000", false},
		{"size", "100", false},
		{"interval", "1", false},
		{"schedule", "normal", false},
		{"clients", "1", false},
		{"inflight", "1", false},
		{"ack-timeout", "30s", false},
		{"connect-concurrency", "16", false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flags := rootCommand.Flags()
			if tt.persistent {
				flags = rootCommand.PersistentFlags()
			}

			flag := flags.Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("flag --%s not found", tt.flag)
			}
			if flag.DefValue != tt.want {
				t.Errorf("--%s default = %q, want %q", tt.flag, flag.DefValue, tt.want)
			}
		})
	}
}

// TestBareCommandRunsPublish checks that the bare command behaves exactly like pub for the same
// flags: same connect options, same publish activity. Both use tiny runs (2 messages, no wait)
// so the check stays fast.
func TestBareCommandRunsPublish(t *testing.T) {
	args := []string{"-c", "2", "-i", "0", "-h", "broker", "-t", "load/bare"}

	bareConnector := &fakeConnector{}
	if _, err := runCommandWithConnector(t, context.Background(), bareConnector, args...); err != nil {
		t.Fatalf("bare command: expected no error, got %v", err)
	}

	pubConnector := &fakeConnector{}
	if _, err := runCommandWithConnector(t, context.Background(), pubConnector, append([]string{"pub"}, args...)...); err != nil {
		t.Fatalf("pub command: expected no error, got %v", err)
	}

	bareCalls := bareConnector.recordedCalls()
	pubCalls := pubConnector.recordedCalls()
	if len(bareCalls) != 1 || len(pubCalls) != 1 {
		t.Fatalf("expected exactly 1 connect call each, got bare=%d pub=%d", len(bareCalls), len(pubCalls))
	}
	if bareCalls[0].options != pubCalls[0].options {
		t.Errorf("bare options = %+v, want %+v", bareCalls[0].options, pubCalls[0].options)
	}

	barePublishes := bareConnector.recordedPublishCalls()
	pubPublishes := pubConnector.recordedPublishCalls()
	if len(barePublishes) != 2 || len(pubPublishes) != 2 {
		t.Fatalf("expected 2 publishes each, got bare=%d pub=%d", len(barePublishes), len(pubPublishes))
	}
	if barePublishes[0].topic != "load/bare" || pubPublishes[0].topic != "load/bare" {
		t.Errorf("expected topic load/bare, got bare=%q pub=%q", barePublishes[0].topic, pubPublishes[0].topic)
	}
}

// TestHostShortFlagEverywhere checks that -h sets host, not help, on both the bare command and
// pub, by checking what the fake connector was actually called with.
func TestHostShortFlagEverywhere(t *testing.T) {
	tests := [][]string{
		{"-h", "broker", "-c", "1", "-i", "0"},
		{"pub", "-h", "broker", "-c", "1", "-i", "0"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			connector := &fakeConnector{}
			if _, err := runCommandWithConnector(t, context.Background(), connector, args...); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := connector.recordedCalls()
			if len(calls) != 1 || calls[0].options.Host != "broker" {
				t.Errorf("expected exactly 1 connect call with host broker, got %+v", calls)
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
		extract   func(options broker.Options) string
		want      string
	}{
		{
			name:     "username",
			envVar:   "MQTT_USERNAME",
			envValue: "envuser",
			extract:  func(options broker.Options) string { return options.Username },
			want:     "envuser",
		},
		{
			name:     "password",
			envVar:   "MQTT_PASSWORD",
			envValue: "envpass",
			extract:  func(options broker.Options) string { return options.Password },
			want:     "envpass",
		},
		{
			name:      "ca",
			envVar:    "MQTT_CA",
			envValue:  "ca.pem",
			extraArgs: []string{"--cert", "cert.pem", "--key", "key.pem"},
			extract:   func(options broker.Options) string { return options.TLSCAFile },
			want:      "ca.pem",
		},
		{
			name:      "cert",
			envVar:    "MQTT_CERT",
			envValue:  "cert.pem",
			extraArgs: []string{"--ca", "ca.pem", "--key", "key.pem"},
			extract:   func(options broker.Options) string { return options.TLSCertFile },
			want:      "cert.pem",
		},
		{
			name:      "key",
			envVar:    "MQTT_KEY",
			envValue:  "key.pem",
			extraArgs: []string{"--ca", "ca.pem", "--cert", "cert.pem"},
			extract:   func(options broker.Options) string { return options.TLSKeyFile },
			want:      "key.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			connector := &fakeConnector{}
			args := append([]string{"-c", "1", "-i", "0"}, tt.extraArgs...)
			if _, err := runCommandWithConnector(t, context.Background(), connector, args...); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := connector.recordedCalls()
			if len(calls) != 1 {
				t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
			}
			if got := tt.extract(calls[0].options); got != tt.want {
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
		extract  func(options broker.Options) string
		want     string
	}{
		{
			name:     "username",
			envVar:   "MQTT_USERNAME",
			envValue: "envuser",
			args:     []string{"--username", "flaguser"},
			extract:  func(options broker.Options) string { return options.Username },
			want:     "flaguser",
		},
		{
			name:     "password",
			envVar:   "MQTT_PASSWORD",
			envValue: "envpass",
			args:     []string{"--password", "flagpass"},
			extract:  func(options broker.Options) string { return options.Password },
			want:     "flagpass",
		},
		{
			name:     "ca",
			envVar:   "MQTT_CA",
			envValue: "envca.pem",
			args:     []string{"--ca", "flagca.pem", "--cert", "cert.pem", "--key", "key.pem"},
			extract:  func(options broker.Options) string { return options.TLSCAFile },
			want:     "flagca.pem",
		},
		{
			name:     "cert",
			envVar:   "MQTT_CERT",
			envValue: "envcert.pem",
			args:     []string{"--ca", "ca.pem", "--cert", "flagcert.pem", "--key", "key.pem"},
			extract:  func(options broker.Options) string { return options.TLSCertFile },
			want:     "flagcert.pem",
		},
		{
			name:     "key",
			envVar:   "MQTT_KEY",
			envValue: "envkey.pem",
			args:     []string{"--ca", "ca.pem", "--cert", "cert.pem", "--key", "flagkey.pem"},
			extract:  func(options broker.Options) string { return options.TLSKeyFile },
			want:     "flagkey.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			connector := &fakeConnector{}
			args := append([]string{"-c", "1", "-i", "0"}, tt.args...)
			if _, err := runCommandWithConnector(t, context.Background(), connector, args...); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := connector.recordedCalls()
			if len(calls) != 1 {
				t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
			}
			if got := tt.extract(calls[0].options); got != tt.want {
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

// TestPasswordReachesConnectOptions checks that --password reaches the options used to connect
// (with no password when it isn't set). Nothing prints the password any more (pub runs for
// real instead of printing its config), so there's no masking left to check.
func TestPasswordReachesConnectOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no password", nil, ""},
		{"password set", []string{"--password", "secret"}, "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := &fakeConnector{}
			args := append([]string{"-c", "1", "-i", "0"}, tt.args...)
			if _, err := runCommandWithConnector(t, context.Background(), connector, args...); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := connector.recordedCalls()
			if len(calls) != 1 || calls[0].options.Password != tt.want {
				t.Errorf("expected password %q, got %+v", tt.want, calls)
			}
		})
	}
}

// alreadyCancelledContext returns a context that is cancelled before it's ever used. sub runs
// until its context is cancelled, so tests that only care about what it dialed with (not the
// reporting loop itself) use this to make it return immediately after subscribing.
func alreadyCancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestSubscribeConnectsWithParsedOptions checks that sub converts its parsed connection flags into
// broker.Options and connects with them, using the default generated client ID. sub no longer
// prints its config (see TestDefaults for pub and dump).
func TestSubscribeConnectsWithParsedOptions(t *testing.T) {
	connector := &fakeConnector{}

	output, err := runCommandWithConnector(t, alreadyCancelledContext(), connector,
		"sub", "-h", "broker", "-p", "1884", "-t", "load/custom", "-q", "2")
	if err != nil {
		t.Fatalf("expected no error, got %v\noutput: %s", err, output)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
	}

	want := broker.Options{
		Host:             "broker",
		Port:             1884,
		CleanSession:     true,
		KeepAliveSeconds: 5,
	}
	if calls[0].options != want {
		t.Errorf("connect options = %+v, want %+v", calls[0].options, want)
	}

	if !strings.HasPrefix(calls[0].clientID, "mqtt-load-generator-") {
		t.Errorf("expected a generated client ID, got %q", calls[0].clientID)
	}
}

// TestSubscribeUsesCustomClientID checks that --clientID is passed straight through instead of a
// generated one.
func TestSubscribeUsesCustomClientID(t *testing.T) {
	connector := &fakeConnector{}

	_, err := runCommandWithConnector(t, alreadyCancelledContext(), connector, "sub", "--clientID", "my-client")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 || calls[0].clientID != "my-client" {
		t.Fatalf("expected a single call with client ID %q, got %+v", "my-client", calls)
	}
}

// TestSubscribeReturnsErrorWhenConnectFails checks that a failed connection is returned as an error
// instead of panicking or exiting the process.
func TestSubscribeReturnsErrorWhenConnectFails(t *testing.T) {
	connector := &fakeConnector{err: errors.New("connection refused")}

	_, err := runCommandWithConnector(t, context.Background(), connector, "sub")
	if err == nil {
		t.Fatal("expected an error, got none")
	}
}

// TestDumpConnectsWithParsedOptions checks that dump converts its parsed connection flags into
// broker.Options and connects with them, using the default generated client ID. dump no longer
// prints its config (see TestDefaults for pub).
func TestDumpConnectsWithParsedOptions(t *testing.T) {
	connector := &fakeConnector{}

	output, err := runCommandWithConnector(t, alreadyCancelledContext(), connector,
		"dump", "-h", "broker", "-p", "1884", "-t", "load/custom", "-q", "2")
	if err != nil {
		t.Fatalf("expected no error, got %v\noutput: %s", err, output)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
	}

	want := broker.Options{
		Host:             "broker",
		Port:             1884,
		CleanSession:     true,
		KeepAliveSeconds: 5,
		Ordered:          true,
	}
	if calls[0].options != want {
		t.Errorf("connect options = %+v, want %+v", calls[0].options, want)
	}

	if !strings.HasPrefix(calls[0].clientID, "mqtt-load-generator-") {
		t.Errorf("expected a generated client ID, got %q", calls[0].clientID)
	}
}

// TestDumpUsesCustomClientID checks that --clientID is passed straight through instead of a
// generated one.
func TestDumpUsesCustomClientID(t *testing.T) {
	connector := &fakeConnector{}

	_, err := runCommandWithConnector(t, alreadyCancelledContext(), connector, "dump", "--clientID", "my-client")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := connector.recordedCalls()
	if len(calls) != 1 || calls[0].clientID != "my-client" {
		t.Fatalf("expected a single call with client ID %q, got %+v", "my-client", calls)
	}
}

// TestDumpReturnsErrorWhenConnectFails checks that a failed connection is returned as an error
// instead of panicking or exiting the process.
func TestDumpReturnsErrorWhenConnectFails(t *testing.T) {
	connector := &fakeConnector{err: errors.New("connection refused")}

	_, err := runCommandWithConnector(t, context.Background(), connector, "dump")
	if err == nil {
		t.Fatal("expected an error, got none")
	}
}

// TestOrderedFlag checks the --ordered defaults (off for sub, on for dump) and that the flag
// reaches the broker options in both directions.
func TestOrderedFlag(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantOrdered bool
	}{
		{"sub default is unordered", []string{"sub"}, false},
		{"sub with --ordered", []string{"sub", "--ordered"}, true},
		{"dump default is ordered", []string{"dump"}, true},
		{"dump with --ordered=false", []string{"dump", "--ordered=false"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := &fakeConnector{}

			if _, err := runCommandWithConnector(t, alreadyCancelledContext(), connector, tt.args...); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := connector.recordedCalls()
			if len(calls) != 1 {
				t.Fatalf("expected exactly 1 connect call, got %d", len(calls))
			}

			if calls[0].options.Ordered != tt.wantOrdered {
				t.Errorf("Ordered = %v, want %v", calls[0].options.Ordered, tt.wantOrdered)
			}
		})
	}
}
