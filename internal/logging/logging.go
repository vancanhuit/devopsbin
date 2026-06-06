// Package logging builds the application's structured logger.
//
// The service always emits JSON-formatted logs via slog so output is
// machine-parseable by log aggregators in every environment.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a slog.Logger that writes JSON-formatted records to w at the
// given level. level is one of "debug", "info", "warn", "error"; unknown
// values fall back to info.
func New(w io.Writer, level string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(handler)
}

// parseLevel maps a textual level to a slog.Level, defaulting to info.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
