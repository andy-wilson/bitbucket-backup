// Package git provides git operations for repository backup.
package git

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LogFunc is called to log debug messages.
type LogFunc func(msg string, args ...interface{})

// CloneMirror performs a mirror clone of a repository.
func CloneMirror(ctx context.Context, repoURL, destPath string) error {
	return CloneMirrorWithLog(ctx, repoURL, destPath, nil)
}

// CloneMirrorWithLog performs a mirror clone with optional logging.
func CloneMirrorWithLog(ctx context.Context, repoURL, destPath string, logFunc LogFunc) error {
	// Mask credentials in URL for logging
	displayURL := maskCredentials(repoURL)

	if logFunc != nil {
		logFunc("Git clone --mirror %s → %s", displayURL, destPath)
	}

	startTime := time.Now()

	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", repoURL, destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone --mirror failed: %w\nOutput: %s", err, string(output))
	}

	if logFunc != nil {
		elapsed := time.Since(startTime)
		size := getDirSize(destPath)
		logFunc("  Clone completed (took %s, %s)", elapsed.Round(time.Millisecond), formatBytes(size))
	}

	return nil
}

// Fetch updates a mirror clone with the latest changes.
func Fetch(ctx context.Context, repoPath string) error {
	return FetchWithLog(ctx, repoPath, nil)
}

// FetchWithLog updates a mirror clone with optional logging.
func FetchWithLog(ctx context.Context, repoPath string, logFunc LogFunc) error {
	if logFunc != nil {
		logFunc("Git fetch --all --prune %s", repoPath)
	}

	startTime := time.Now()
	sizeBefore := getDirSize(repoPath)

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--all", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nOutput: %s", err, string(output))
	}

	if logFunc != nil {
		elapsed := time.Since(startTime)
		sizeAfter := getDirSize(repoPath)
		delta := sizeAfter - sizeBefore
		deltaStr := ""
		if delta > 0 {
			deltaStr = fmt.Sprintf(", +%s", formatBytes(delta))
		} else if delta < 0 {
			deltaStr = fmt.Sprintf(", %s", formatBytes(delta))
		}
		logFunc("  Fetch completed (took %s, %s%s)", elapsed.Round(time.Millisecond), formatBytes(sizeAfter), deltaStr)
	}

	return nil
}

// AuthenticatedURL adds credentials to a git URL.
// For HTTPS URLs, embeds the username and password.
// Returns the original URL for non-HTTPS URLs.
func AuthenticatedURL(repoURL, username, password string) string {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return repoURL
	}

	if parsed.Scheme != "https" {
		return repoURL
	}

	// Set credentials
	parsed.User = url.UserPassword(username, password)
	return parsed.String()
}

// IsGitInstalled checks if git is available in PATH.
func IsGitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// GetVersion returns the installed git version.
func GetVersion() (string, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting git version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Fsck runs git fsck to verify repository integrity.
func Fsck(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fsck", "--full")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fsck failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// maskCredentials removes credentials from a URL for safe logging.
func maskCredentials(repoURL string) string {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return repoURL
	}
	if parsed.User != nil {
		parsed.User = url.User("***")
	}
	return parsed.String()
}

// getDirSize returns the total size of a directory in bytes.
func getDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentionally continue walking on errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMG"[exp])
}
