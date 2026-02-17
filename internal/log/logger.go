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

	// Collect all output writers — stdout is always included.
	writers := []io.Writer{os.Stdout}

	// File output
	if cfg.Outputs.File.Enabled {
		w, err := createFileWriter(cfg.Outputs.File)
		if err != nil {
			return fmt.Errorf("failed to create file output: %w", err)
		}
		writers = append(writers, w)
	}

	// Loki output
	if cfg.Outputs.Loki.Enabled {
		w, err := createLokiWriter(cfg.Outputs.Loki)
		if err != nil {
			return fmt.Errorf("failed to create loki output: %w", err)
		}
		writers = append(writers, w)
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

// createFileWriter creates a lumberjack file writer for log rotation.
func createFileWriter(fc config.FileOutputConfig) (io.Writer, error) {
	if fc.Path == "" {
		return nil, fmt.Errorf("file output requires 'path' field")
	}
	return &lumberjack.Logger{
		Filename:   fc.Path,
		MaxSize:    fc.Rotation.MaxSizeMB,
		MaxBackups: fc.Rotation.MaxBackups,
		MaxAge:     fc.Rotation.MaxAgeDays,
		Compress:   fc.Rotation.Compress,
	}, nil
}

// createLokiWriter creates a Loki writer.
func createLokiWriter(lc config.LokiOutputConfig) (io.Writer, error) {
	if lc.Endpoint == "" {
		return nil, fmt.Errorf("loki output requires 'endpoint' field")
	}
	return NewLokiWriter(LokiConfig{
		Endpoint:      lc.Endpoint,
		Labels:        lc.Labels,
		BatchSize:     lc.BatchSize,
		FlushInterval: lc.BatchTimeout,
	})
}
