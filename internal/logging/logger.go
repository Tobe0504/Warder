package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options configures the application logger.
type Options struct {
	// Level is the minimum level emitted.
	Level slog.Level

	// JSON selects machine-readable output, which production should use so
	// that logs can be shipped and queried without reparsing.
	JSON bool

	// Output defaults to stderr.
	Output io.Writer
}

// New builds the application logger with redaction always applied.
//
// There is no option to disable redaction. A debug flag that turns off
// credential redaction is the flag that gets set during an incident, at the
// exact moment logs are being copied into a chat window.
func New(opts Options) *slog.Logger {
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{Level: opts.Level}

	var base slog.Handler
	if opts.JSON {
		base = slog.NewJSONHandler(out, handlerOpts)
	} else {
		base = slog.NewTextHandler(out, handlerOpts)
	}

	return slog.New(NewHandler(base))
}

// ParseLevel converts a configured level name, defaulting to info.
func ParseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
