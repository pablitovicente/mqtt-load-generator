package cli

import (
	"io"
	"log/slog"
)

// newLogger builds the logger a command runs with: JSON on output (stderr in production), at
// the level named by levelName. Connection.Validate has already checked that levelName is one
// of the four names slog knows about by the time this is called.
func newLogger(levelName string, output io.Writer) *slog.Logger {
	var level slog.Level

	switch levelName {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}
