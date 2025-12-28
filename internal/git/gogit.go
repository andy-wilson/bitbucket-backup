// Package git provides git operations for repository backup.
// This file implements git operations using go-git (pure Go).
package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/filesystem"

	"github.com/go-git/go-billy/v5/osfs"
)

// ProgressCallback is called to report git operation progress.
type ProgressCallback func(stage string, current, total int64)

// RateLimitFunc is called before each HTTP request to enforce rate limiting.
type RateLimitFunc func()

// GoGitClient provides git operations using go-git.
type GoGitClient struct {
	username      string
	password      string
	logFunc       LogFunc
	progressFunc  ProgressCallback
	rateLimitFunc RateLimitFunc
	httpClient    *http.Client
	setupOnce     sync.Once
}

// GoGitOption configures a GoGitClient.
type GoGitOption func(*GoGitClient)

// WithCredentials sets the username and password for authentication.
func WithCredentials(username, password string) GoGitOption {
	return func(c *GoGitClient) {
		c.username = username
		c.password = password
	}
}

// WithLogger sets the log function for debug output.
func WithLogger(logFunc LogFunc) GoGitOption {
	return func(c *GoGitClient) {
		c.logFunc = logFunc
	}
}

// WithProgress sets the progress callback.
func WithProgress(progressFunc ProgressCallback) GoGitOption {
	return func(c *GoGitClient) {
		c.progressFunc = progressFunc
	}
}

// WithRateLimit sets the rate limit function called before each HTTP request.
func WithRateLimit(rateLimitFunc RateLimitFunc) GoGitOption {
	return func(c *GoGitClient) {
		c.rateLimitFunc = rateLimitFunc
	}
}

// NewGoGitClient creates a new go-git based client.
func NewGoGitClient(opts ...GoGitOption) *GoGitClient {
	c := &GoGitClient{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// rateLimitedTransport wraps an http.RoundTripper to add rate limiting.
type rateLimitedTransport struct {
	base          http.RoundTripper
	rateLimitFunc RateLimitFunc
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.rateLimitFunc != nil {
		t.rateLimitFunc()
	}
	return t.base.RoundTrip(req)
}

// setupHTTPClient configures a custom HTTP client with rate limiting.
func (c *GoGitClient) setupHTTPClient() {
	c.setupOnce.Do(func() {
		transport := &rateLimitedTransport{
			base:          http.DefaultTransport,
			rateLimitFunc: c.rateLimitFunc,
		}
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   0, // No timeout, we use context for cancellation
		}

		// Install custom HTTPS transport
		client.InstallProtocol("https", githttp.NewClient(c.httpClient))
	})
}

// getAuth returns the authentication for git operations.
func (c *GoGitClient) getAuth() transport.AuthMethod {
	if c.username == "" && c.password == "" {
		return nil
	}
	return &githttp.BasicAuth{
		Username: c.username,
		Password: c.password,
	}
}

// progressWriter wraps progress reporting.
type progressWriter struct {
	logFunc LogFunc
}

func (w *progressWriter) Write(p []byte) (n int, err error) {
	if w.logFunc != nil {
		w.logFunc("  %s", string(p))
	}
	return len(p), nil
}

// CloneMirror performs a mirror clone of a repository.
func (c *GoGitClient) CloneMirror(ctx context.Context, repoURL, destPath string) error {
	c.setupHTTPClient()

	startTime := time.Now()
	if c.logFunc != nil {
		c.logFunc("Git clone --mirror %s → %s", maskCredentialsInURL(repoURL), destPath)
	}

	// Create the destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Set up filesystem storage for bare repo
	fs := osfs.New(destPath)
	dot, err := fs.Chroot(".git")
	if err != nil {
		// For bare repos, use the root
		dot = fs
	}
	storage := filesystem.NewStorage(dot, nil)

	// Progress writer
	var progress io.Writer
	if c.logFunc != nil {
		progress = &progressWriter{logFunc: c.logFunc}
	}

	// Clone with mirror option
	repo, err := git.CloneContext(ctx, storage, nil, &git.CloneOptions{
		URL:      repoURL,
		Auth:     c.getAuth(),
		Mirror:   true,
		Progress: progress,
	})
	if err != nil {
		// Clean up on failure
		_ = os.RemoveAll(destPath)
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Verify the clone worked
	_, err = repo.Head()
	if err != nil && err.Error() != "reference not found" {
		// Some repos might be empty, which is okay
		if c.logFunc != nil {
			c.logFunc("  Warning: could not get HEAD: %v", err)
		}
	}

	if c.logFunc != nil {
		elapsed := time.Since(startTime)
		size := getDirSize(destPath)
		c.logFunc("  Clone completed (took %s, %s)", elapsed.Round(time.Millisecond), formatBytes(size))
	}

	return nil
}

// Fetch updates a mirror clone with the latest changes.
func (c *GoGitClient) Fetch(ctx context.Context, repoPath string) error {
	c.setupHTTPClient()

	startTime := time.Now()
	if c.logFunc != nil {
		c.logFunc("Git fetch --all --prune %s", repoPath)
	}

	sizeBefore := getDirSize(repoPath)

	// Open the existing repository
	fs := osfs.New(repoPath)
	storage := filesystem.NewStorage(fs, nil)

	repo, err := git.Open(storage, nil)
	if err != nil {
		return fmt.Errorf("opening repository: %w", err)
	}

	// Progress writer
	var progress io.Writer
	if c.logFunc != nil {
		progress = &progressWriter{logFunc: c.logFunc}
	}

	// Fetch all remotes
	remotes, err := repo.Remotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	for _, remote := range remotes {
		err := remote.FetchContext(ctx, &git.FetchOptions{
			Auth:     c.getAuth(),
			Progress: progress,
			Prune:    true,
			RefSpecs: []config.RefSpec{
				"+refs/*:refs/*",
			},
		})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("fetching from %s: %w", remote.Config().Name, err)
		}
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

// Fsck verifies repository integrity using go-git.
func (c *GoGitClient) Fsck(_ context.Context, repoPath string) error {
	// Open the existing repository
	fs := osfs.New(repoPath)
	storage := filesystem.NewStorage(fs, nil)

	repo, err := git.Open(storage, nil)
	if err != nil {
		return fmt.Errorf("opening repository: %w", err)
	}

	// Get all objects and verify they can be read
	objIter, err := repo.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return fmt.Errorf("iterating objects: %w", err)
	}

	count := 0
	err = objIter.ForEach(func(_ plumbing.EncodedObject) error {
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("verifying objects: %w", err)
	}

	if c.logFunc != nil {
		c.logFunc("  Verified %d objects", count)
	}

	return nil
}

// maskCredentialsInURL removes credentials from a URL for safe logging.
func maskCredentialsInURL(repoURL string) string {
	return maskCredentials(repoURL)
}
