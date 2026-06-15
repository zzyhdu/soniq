package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	LogFormatJSON = "json"
	LogFormatText = "text"

	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// LoggerConfig configures a process-wide structured logger.
type LoggerConfig struct {
	Service string
	Format  string
	Level   string
	Output  io.Writer
}

// NewLogger builds a slog logger with stable Soniq defaults.
func NewLogger(config LoggerConfig) (*slog.Logger, error) {
	format := strings.ToLower(strings.TrimSpace(config.Format))
	if format == "" {
		format = LogFormatText
	}
	level, err := parseLogLevel(config.Level)
	if err != nil {
		return nil, err
	}
	output := config.Output
	if output == nil {
		output = os.Stdout
	}
	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(output, options)
	case LogFormatText:
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("unsupported log format %q", config.Format)
	}

	logger := slog.New(handler)
	if service := strings.TrimSpace(config.Service); service != "" {
		logger = logger.With(slog.String("service", service))
	}
	return logger, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", LogLevelInfo:
		return slog.LevelInfo, nil
	case LogLevelDebug:
		return slog.LevelDebug, nil
	case LogLevelWarn:
		return slog.LevelWarn, nil
	case LogLevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q", raw)
	}
}
