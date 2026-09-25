package cli

import (
	"io"
	"log/slog"
)

// parseLogLevel turns a --log-level name (debug, info, warn, error; any case) into a
// slog.Level, using slog's own parser.
func parseLogLevel(name string) (slog.Level, error) {
	var level slog.Level
	err := level.UnmarshalText([]byte(name))
	return level, err
}

// newLogger builds the logger a command runs with: JSON on output (stderr in production), at
// the level named by levelName. Connection.Validate has already rejected unknown names, so an
// error here can't happen and the level falls back to info.
func newLogger(levelName string, output io.Writer) *slog.Logger {
	level, _ := parseLogLevel(levelName)

	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}
