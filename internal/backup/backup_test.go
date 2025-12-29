package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestIsContextCanceled(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, true},
		{"context deadline", context.DeadlineExceeded, true},
		{"other error", errors.New("some error"), false},
		{"wrapped context canceled", errors.New("operation failed: context canceled"), true},
		{"wrapped deadline", errors.New("timeout: context deadline exceeded"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isContextCanceled(tt.err)
			if got != tt.want {
				t.Errorf("isContextCanceled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake bare repo with HEAD file
	bareRepoDir := filepath.Join(tmpDir, "bare-repo")
	if err := os.MkdirAll(bareRepoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareRepoDir, "HEAD"), []byte("ref: refs/heads/main"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a fake working repo with .git/HEAD file
	workingRepoDir := filepath.Join(tmpDir, "working-repo")
	gitDir := filepath.Join(workingRepoDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"bare repo", bareRepoDir, true},
		{"working repo", workingRepoDir, true},
		{"non-existent", filepath.Join(tmpDir, "nonexistent"), false},
		{"empty dir", tmpDir, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidGitRepo(tt.path)
			if got != tt.want {
				t.Errorf("isValidGitRepo(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDefaultLogger(t *testing.T) {
	l := &defaultLogger{quiet: false}

	// These should not panic
	l.Info("info message")
	l.Debug("debug message")
	l.Error("error message")
}

func TestDefaultLogger_Quiet(t *testing.T) {
	l := &defaultLogger{quiet: true}

	// In quiet mode, these should not panic
	l.Info("info message")
	l.Debug("debug message")
	l.Error("error message")
}
