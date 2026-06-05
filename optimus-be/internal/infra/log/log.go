package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level  string
	Format string
	Writer io.Writer
}

func New(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	level := parseLevel(opts.Level)
	var handler slog.Handler
	hOpts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(opts.Format, "text") {
		handler = slog.NewTextHandler(w, hOpts)
	} else {
		handler = slog.NewJSONHandler(w, hOpts)
	}
	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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
