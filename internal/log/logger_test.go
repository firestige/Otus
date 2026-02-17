package log

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"firestige.xyz/otus/internal/config"
)

func TestParseLevelValid(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := parseLevel(tt.input)
			if err != nil {
				t.Errorf("parseLevel(%q) returned error: %v", tt.input, err)
			}
			if level != tt.expected {
				t.Errorf("parseLevel(%q) = %v, expected %v", tt.input, level, tt.expected)
			}
		})
	}
}

func TestParseLevelInvalid(t *testing.T) {
	tests := []string{"invalid", "trace", "fatal", ""}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseLevel(input)
			if err == nil {
				t.Errorf("parseLevel(%q) should return error, got nil", input)
			}
		})
	}
}

func TestInitStdoutOnly(t *testing.T) {
	cfg := config.LogConfig{
		Level:  "info",
		Format: "json",
		// No file or loki enabled → stdout only
	}

	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := slog.Default()
	if logger == nil {
		t.Fatal("Expected logger to be set, got nil")
	}
}

func TestInitWithFileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := config.LogConfig{
		Level:  "debug",
		Format: "text",
		Outputs: config.LogOutputsConfig{
			File: config.FileOutputConfig{
				Enabled: true,
				Path:    logPath,
				Rotation: config.RotationConfig{
					MaxSizeMB:  10,
					MaxBackups: 3,
					MaxAgeDays: 7,
					Compress:   true,
				},
			},
		},
	}

	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Write a log message
	slog.Info("test message", "key", "value")

	// Verify log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logPath)
	}
}

func TestInitWithInvalidLevel(t *testing.T) {
	cfg := config.LogConfig{
		Level:  "invalid",
		Format: "json",
	}

	err := Init(cfg)
	if err == nil {
		t.Error("Expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Errorf("Expected error about invalid log level, got: %v", err)
	}
}

func TestInitWithInvalidFormat(t *testing.T) {
	cfg := config.LogConfig{
		Level:  "info",
		Format: "xml",
	}

	err := Init(cfg)
	if err == nil {
		t.Error("Expected error for invalid log format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported log format") {
		t.Errorf("Expected error about unsupported format, got: %v", err)
	}
}

func TestInitWithMissingFilePath(t *testing.T) {
	cfg := config.LogConfig{
		Level:  "info",
		Format: "json",
		Outputs: config.LogOutputsConfig{
			File: config.FileOutputConfig{
				Enabled: true,
				// Missing Path
			},
		},
	}

	err := Init(cfg)
	if err == nil {
		t.Error("Expected error for missing file path, got nil")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("Expected error about missing path, got: %v", err)
	}
}

func TestCreateFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	fc := config.FileOutputConfig{
		Enabled: true,
		Path:    filepath.Join(tmpDir, "test.log"),
		Rotation: config.RotationConfig{
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   true,
		},
	}

	writer, err := createFileWriter(fc)
	if err != nil {
		t.Fatalf("createFileWriter failed: %v", err)
	}
	if writer == nil {
		t.Fatal("Expected writer, got nil")
	}

	// Write something to verify it works
	n, err := writer.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("Expected 4 bytes written, got %d", n)
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	// Create logger with WARN level
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})
	logger := slog.New(handler)

	// Log messages at different levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	// Debug and Info should be filtered out
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info message should be filtered out")
	}

	// Warn and Error should be present
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should be present")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message should be present")
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	logger.Info("test message", "key", "value", "number", 42)

	output := buf.String()

	// Check JSON format
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Error("JSON output should contain message field")
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Error("JSON output should contain key field")
	}
	if !strings.Contains(output, `"number":42`) {
		t.Error("JSON output should contain number field")
	}
}

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	logger.Info("test message", "key", "value")

	output := buf.String()

	// Check text format
	if !strings.Contains(output, "test message") {
		t.Error("Text output should contain message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("Text output should contain key=value")
	}
}
