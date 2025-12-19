// Package git provides git operations for repository backup.
package git

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// CloneMirror performs a mirror clone of a repository.
func CloneMirror(ctx context.Context, repoURL, destPath string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", repoURL, destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone --mirror failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Fetch updates a mirror clone with the latest changes.
func Fetch(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--all", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nOutput: %s", err, string(output))
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
