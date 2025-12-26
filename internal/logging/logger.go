// Package logging provides a configurable logger for bb-backup.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents a log level.
type Level int

// Log levels.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a log level string.
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger is a configurable logger.
type Logger struct {
	mu      sync.Mutex
	level   Level
	format  string // "text" or "json"
	output  io.Writer
	file    *os.File // Keep reference to close later
	console bool     // Also write to console
}

// Config holds logger configuration.
type Config struct {
	Level   string // "debug", "info", "warn", "error"
	Format  string // "text" or "json"
	File    string // Log file path (empty for console only)
	Console bool   // Also write to console when file is set
}

// New creates a new logger from configuration.
func New(cfg Config) (*Logger, error) {
	l := &Logger{
		level:   ParseLevel(cfg.Level),
		format:  cfg.Format,
		output:  os.Stdout,
		console: cfg.Console,
	}

	if cfg.File != "" {
		// Add timestamp to filename to avoid overwriting previous logs
		logFile := addTimestampToFilename(cfg.File)

		// Ensure directory exists
		dir := filepath.Dir(logFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating log directory: %w", err)
		}

		// Open log file (create new file for each run)
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
		l.file = f

		if cfg.Console {
			// Write to both file and console
			l.output = io.MultiWriter(f, os.Stdout)
		} else {
			l.output = f
		}

		// Log the filename being used (to console if also logging there)
		fmt.Fprintf(os.Stderr, "Logging to: %s\n", logFile)
	}

	return l, nil
}

// addTimestampToFilename inserts a timestamp before the file extension.
// e.g., "bb-backup.log" -> "bb-backup-2025-12-26T22-15-30Z.log"
func addTimestampToFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := filename[:len(filename)-len(ext)]
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s-%s%s", base, timestamp, ext)
}

// Close closes the log file if open.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a log message at the given level.
func (l *Logger) log(level Level, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	formatted := fmt.Sprintf(msg, args...)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	if l.format == "json" {
		entry := map[string]interface{}{
			"timestamp": timestamp,
			"level":     level.String(),
			"message":   formatted,
		}
		data, _ := json.Marshal(entry)
		_, _ = fmt.Fprintln(l.output, string(data))
	} else {
		_, _ = fmt.Fprintf(l.output, "%s [%s] %s\n", timestamp, level.String(), formatted)
	}

	// Flush file to disk to ensure logs are written immediately
	if l.file != nil {
		_ = l.file.Sync()
	}

	// For errors, also write to stderr if we're logging to a file
	if level == LevelError && l.file != nil && !l.console {
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", formatted)
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

// IsDebug returns true if debug logging is enabled.
func (l *Logger) IsDebug() bool {
	return l.level <= LevelDebug
}

// IsQuiet returns true if only errors are logged.
func (l *Logger) IsQuiet() bool {
	return l.level >= LevelError
}
