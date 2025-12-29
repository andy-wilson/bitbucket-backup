// Package git provides git operations for repository backup.
// This file implements git operations using the shell git CLI as a fallback.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellGitClient provides git operations using the git CLI.
type ShellGitClient struct {
	username string
	password string
	logFunc  LogFunc
	gitPath  string
}

// ShellGitOption configures a ShellGitClient.
type ShellGitOption func(*ShellGitClient)

// WithShellCredentials sets the username and password for authentication.
func WithShellCredentials(username, password string) ShellGitOption {
	return func(c *ShellGitClient) {
		c.username = username
		c.password = password
	}
}

// WithShellLogger sets the log function for debug output.
func WithShellLogger(logFunc LogFunc) ShellGitOption {
	return func(c *ShellGitClient) {
		c.logFunc = logFunc
	}
}

// NewShellGitClient creates a new shell git based client.
// Returns nil if git is not available.
func NewShellGitClient(opts ...ShellGitOption) *ShellGitClient {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil
	}

	c := &ShellGitClient{
		gitPath: gitPath,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// IsAvailable returns true if git CLI is available.
func IsGitCLIAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// buildAuthURL creates an authenticated URL for git operations.
func (c *ShellGitClient) buildAuthURL(repoURL string) string {
	if c.username == "" || c.password == "" {
		return repoURL
	}

	// Insert credentials into URL
	// https://bitbucket.org/... -> https://user:pass@bitbucket.org/...
	if strings.HasPrefix(repoURL, "https://") {
		return fmt.Sprintf("https://%s:%s@%s",
			c.username,
			c.password,
			strings.TrimPrefix(repoURL, "https://"))
	}
	return repoURL
}

// CloneMirror performs a mirror clone of a repository using git CLI.
func (c *ShellGitClient) CloneMirror(ctx context.Context, repoURL, destPath string) error {
	startTime := time.Now()
	if c.logFunc != nil {
		c.logFunc("Git CLI clone --mirror %s → %s", maskCredentials(repoURL), destPath)
	}

	// Build authenticated URL
	authURL := c.buildAuthURL(repoURL)

	// Run git clone --mirror
	cmd := exec.CommandContext(ctx, c.gitPath, "clone", "--mirror", authURL, destPath)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // Disable interactive prompts
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Clean up on failure
		_ = os.RemoveAll(destPath)
		return fmt.Errorf("git clone failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	if c.logFunc != nil {
		elapsed := time.Since(startTime)
		size := getDirSize(destPath)
		c.logFunc("  Clone completed (took %s, %s)", elapsed.Round(time.Millisecond), formatBytes(size))
	}

	return nil
}

// Fetch updates a mirror clone with the latest changes using git CLI.
func (c *ShellGitClient) Fetch(ctx context.Context, repoPath string) error {
	startTime := time.Now()
	if c.logFunc != nil {
		c.logFunc("Git CLI fetch --all --prune %s", repoPath)
	}

	sizeBefore := getDirSize(repoPath)

	// Run git fetch --all --prune
	cmd := exec.CommandContext(ctx, c.gitPath, "-C", repoPath, "fetch", "--all", "--prune")
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // Disable interactive prompts
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	if c.logFunc != nil {
		elapsed := time.Since(startTime)
		sizeAfter := getDirSize(repoPath)
		delta := sizeAfter - sizeBefore
		deltaStr := ""
		if delta > 0 {
			deltaStr = fmt.Sprintf(", +%s", formatBytes(delta))
		} else if delta < 0 {
			deltaStr = fmt.Sprintf(", %s", formatBytes(delta))
		}
		c.logFunc("  Fetch completed (took %s, %s%s)", elapsed.Round(time.Millisecond), formatBytes(sizeAfter), deltaStr)
	}

	return nil
}

// Fsck verifies repository integrity using git CLI.
func (c *ShellGitClient) Fsck(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, c.gitPath, "-C", repoPath, "fsck", "--no-dangling")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("git fsck failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	if c.logFunc != nil {
		c.logFunc("  Repository verified with git fsck")
	}

	return nil
}
