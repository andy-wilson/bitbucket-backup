package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Level.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"unknown", LevelInfo}, // Default to info
		{"", LevelInfo},        // Default to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewLogger_ConsoleOnly(t *testing.T) {
	cfg := Config{
		Level:  "info",
		Format: "text",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	if logger.level != LevelInfo {
		t.Errorf("level = %v, want %v", logger.level, LevelInfo)
	}
	if logger.file != nil {
		t.Error("file should be nil for console-only logger")
	}
}

func TestNewLogger_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:   "debug",
		Format:  "text",
		File:    logFile,
		Console: false,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	if logger.level != LevelDebug {
		t.Errorf("level = %v, want %v", logger.level, LevelDebug)
	}
	if logger.file == nil {
		t.Error("file should not be nil when file path is set")
	}

	// Check that a timestamped log file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 log file, got %d", len(files))
	}
	if !strings.HasPrefix(files[0].Name(), "test-") {
		t.Errorf("log file name should start with 'test-', got %s", files[0].Name())
	}
}

func TestNewLogger_WithFileAndConsole(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:   "info",
		Format:  "text",
		File:    logFile,
		Console: true,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	if !logger.console {
		t.Error("console should be true")
	}
}

func TestNewLogger_InvalidDirectory(t *testing.T) {
	// Use a path that can't be created (file as directory)
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("block"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := Config{
		File: filepath.Join(blockingFile, "subdir", "test.log"),
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New() should fail when directory can't be created")
	}
}

func TestLogger_LogLevels(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		level:  LevelDebug,
		format: "text",
		output: &buf,
	}

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	if !strings.Contains(output, "[DEBUG] debug message") {
		t.Error("missing debug message")
	}
	if !strings.Contains(output, "[INFO] info message") {
		t.Error("missing info message")
	}
	if !strings.Contains(output, "[WARN] warn message") {
		t.Error("missing warn message")
	}
	if !strings.Contains(output, "[ERROR] error message") {
		t.Error("missing error message")
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		level:  LevelWarn,
		format: "text",
		output: &buf,
	}

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("debug message should be filtered")
	}
	if strings.Contains(output, "info message") {
		t.Error("info message should be filtered")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("warn message should be present")
	}
	if !strings.Contains(output, "error message") {
		t.Error("error message should be present")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		level:  LevelInfo,
		format: "json",
		output: &buf,
	}

	logger.Info("test message")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("JSON unmarshal error = %v", err)
	}

	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["message"] != "test message" {
		t.Errorf("message = %v, want 'test message'", entry["message"])
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("missing timestamp field")
	}
}

func TestLogger_FormatArgs(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		level:  LevelInfo,
		format: "text",
		output: &buf,
	}

	logger.Info("count: %d, name: %s", 42, "test")

	output := buf.String()
	if !strings.Contains(output, "count: 42, name: test") {
		t.Errorf("formatted message not found in output: %s", output)
	}
}

func TestLogger_IsDebug(t *testing.T) {
	tests := []struct {
		level    Level
		expected bool
	}{
		{LevelDebug, true},
		{LevelInfo, false},
		{LevelWarn, false},
		{LevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			logger := &Logger{level: tt.level}
			if got := logger.IsDebug(); got != tt.expected {
				t.Errorf("IsDebug() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogger_IsQuiet(t *testing.T) {
	tests := []struct {
		level    Level
		expected bool
	}{
		{LevelDebug, false},
		{LevelInfo, false},
		{LevelWarn, false},
		{LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			logger := &Logger{level: tt.level}
			if got := logger.IsQuiet(); got != tt.expected {
				t.Errorf("IsQuiet() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogger_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level: "info",
		File:  logFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("test message")

	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Close should be idempotent (file already closed)
	// Calling close again on nil file should not error
	logger.file = nil
	if err := logger.Close(); err != nil {
		t.Errorf("Close() on nil file error = %v", err)
	}
}

func TestAddTimestampToFilename(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"test.log", "test-"},
		{"path/to/file.log", "path/to/file-"},
		{"noext", "noext-"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := addTimestampToFilename(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("addTimestampToFilename(%q) = %q, should contain %q", tt.input, result, tt.contains)
			}
			// Check that it has a timestamp-like pattern
			if !strings.Contains(result, "202") { // Year prefix
				t.Errorf("addTimestampToFilename(%q) = %q, should contain timestamp", tt.input, result)
			}
		})
	}
}

func TestLogger_ConcurrentAccess(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		level:  LevelInfo,
		format: "text",
		output: &buf,
	}

	// Test concurrent logging doesn't panic
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				logger.Info("goroutine %d message %d", n, j)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no panic occurred and some output was written
	if buf.Len() == 0 {
		t.Error("expected some output from concurrent logging")
	}
}
