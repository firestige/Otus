// Package log implements structured logging using slog.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"firestige.xyz/otus/internal/config"
)

// Init initializes the global logger based on configuration.
func Init(cfg config.LogConfig) error {
	// Parse log level
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	// Collect all output writers
	var writers []io.Writer
	for i, output := range cfg.Outputs {
		writer, err := createWriter(output)
		if err != nil {
			return fmt.Errorf("failed to create output[%d] (%s): %w", i, output.Type, err)
		}
		if writer != nil {
			writers = append(writers, writer)
		}
	}

	// Default to stdout if no outputs configured
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	// Create multi-writer
	multiWriter := io.MultiWriter(writers...)

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(multiWriter, opts)
	case "text":
		handler = slog.NewTextHandler(multiWriter, opts)
	default:
		return fmt.Errorf("unsupported log format: %s (must be json or text)", cfg.Format)
	}

	// Set global logger
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

// parseLevel converts string level to slog.Level.
func parseLevel(levelStr string) (slog.Level, error) {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown level: %s", levelStr)
	}
}

// createWriter creates an io.Writer for the given output config.
func createWriter(output config.OutputConfig) (io.Writer, error) {
	switch strings.ToLower(output.Type) {
	case "console", "stdout":
		return os.Stdout, nil

	case "file":
		if output.Path == "" {
			return nil, fmt.Errorf("file output requires 'path' field")
		}

		// Use lumberjack for log rotation
		return &lumberjack.Logger{
			Filename:   output.Path,
			MaxSize:    output.MaxSizeMB,    // megabytes
			MaxBackups: output.MaxBackups,   // number of old files to keep
			MaxAge:     output.MaxAgeDays,   // days
			Compress:   output.Compress,     // compress old files
		}, nil

	case "loki":
		if output.Endpoint == "" {
			return nil, fmt.Errorf("loki output requires 'endpoint' field")
		}

		// Create Loki writer
		lokiWriter, err := NewLokiWriter(LokiConfig{
			Endpoint:      output.Endpoint,
			Labels:        output.Labels,
			BatchSize:     output.BatchSize,
			FlushInterval: output.FlushInterval,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create loki writer: %w", err)
		}

		return lokiWriter, nil

	default:
		return nil, fmt.Errorf("unsupported output type: %s", output.Type)
	}
}
