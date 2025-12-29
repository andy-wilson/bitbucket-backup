package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAuthenticatedURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		username string
		password string
		want     string
	}{
		{
			name:     "https url",
			url:      "https://bitbucket.org/workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "https://user:pass@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "https url with special chars in password",
			url:      "https://bitbucket.org/workspace/repo.git",
			username: "user",
			password: "p@ss:word",
			want:     "https://user:p%40ss%3Aword@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "ssh url unchanged",
			url:      "git@bitbucket.org:workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "git@bitbucket.org:workspace/repo.git",
		},
		{
			name:     "http url",
			url:      "http://bitbucket.org/workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "http://bitbucket.org/workspace/repo.git", // Only https gets auth
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AuthenticatedURL(tt.url, tt.username, tt.password)
			if got != tt.want {
				t.Errorf("AuthenticatedURL() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestIsGitInstalled(t *testing.T) {
	// This test depends on the environment, but git should be installed
	// for development. Skip if not available.
	if !IsGitInstalled() {
		t.Skip("git not installed, skipping test")
	}

	// If we get here, git is installed
	if !IsGitInstalled() {
		t.Error("IsGitInstalled should return true")
	}
}

func TestGetVersion(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed, skipping test")
	}

	version, err := GetVersion()
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}

	if version == "" {
		t.Error("expected non-empty version string")
	}

	// Version should start with "git version"
	if len(version) < 11 || version[:11] != "git version" {
		t.Errorf("unexpected version format: %s", version)
	}
}

func TestShellGitBuildAuthURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		username string
		password string
		want     string
	}{
		{
			name:     "plain https url",
			url:      "https://bitbucket.org/workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "https://user:pass@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "url with existing username (Bitbucket API format)",
			url:      "https://existinguser@bitbucket.org/workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "https://user:pass@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "url with existing user:pass credentials",
			url:      "https://olduser:oldpass@bitbucket.org/workspace/repo.git",
			username: "newuser",
			password: "newpass",
			want:     "https://newuser:newpass@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "no credentials provided",
			url:      "https://existinguser@bitbucket.org/workspace/repo.git",
			username: "",
			password: "",
			want:     "https://existinguser@bitbucket.org/workspace/repo.git",
		},
		{
			name:     "ssh url unchanged",
			url:      "git@bitbucket.org:workspace/repo.git",
			username: "user",
			password: "pass",
			want:     "git@bitbucket.org:workspace/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &ShellGitClient{
				username: tt.username,
				password: tt.password,
			}
			got := client.buildAuthURL(tt.url)
			if got != tt.want {
				t.Errorf("buildAuthURL() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMaskCredentials(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "url with credentials",
			input: "https://user:pass@bitbucket.org/repo.git",
			want:  "https://%2A%2A%2A@bitbucket.org/repo.git", // URL-encoded ***
		},
		{
			name:  "url with username only",
			input: "https://user@bitbucket.org/repo.git",
			want:  "https://%2A%2A%2A@bitbucket.org/repo.git", // URL-encoded ***
		},
		{
			name:  "url without credentials",
			input: "https://bitbucket.org/repo.git",
			want:  "https://bitbucket.org/repo.git",
		},
		{
			name:  "invalid url",
			input: "not-a-url",
			want:  "not-a-url",
		},
		{
			name:  "ssh url",
			input: "git@bitbucket.org:workspace/repo.git",
			want:  "git@bitbucket.org:workspace/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskCredentials(tt.input)
			if got != tt.want {
				t.Errorf("maskCredentials(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

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

func TestGetDirSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "subdir", "file2.txt")

	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(file2), 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(file2, []byte("world!"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	size := getDirSize(tmpDir)
	// "hello" (5) + "world!" (6) = 11
	if size != 11 {
		t.Errorf("getDirSize() = %d, want 11", size)
	}
}

func TestGetDirSize_NonExistent(t *testing.T) {
	size := getDirSize("/nonexistent/path")
	if size != 0 {
		t.Errorf("getDirSize(nonexistent) = %d, want 0", size)
	}
}

func TestNewShellGitClient(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	client := NewShellGitClient()
	if client == nil {
		t.Error("NewShellGitClient() returned nil when git is installed")
	}
	if client.gitPath == "" {
		t.Error("gitPath should not be empty")
	}
}

func TestNewShellGitClient_WithOptions(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	logFunc := func(msg string, args ...interface{}) {
		// Log function set
	}

	client := NewShellGitClient(
		WithShellCredentials("user", "pass"),
		WithShellLogger(logFunc),
	)

	if client == nil {
		t.Fatal("NewShellGitClient() returned nil")
	}
	if client.username != "user" {
		t.Errorf("username = %q, want %q", client.username, "user")
	}
	if client.password != "pass" {
		t.Errorf("password = %q, want %q", client.password, "pass")
	}
	if client.logFunc == nil {
		t.Error("logFunc should not be nil")
	}
}

func TestIsGitCLIAvailable(t *testing.T) {
	// Test that the function doesn't panic
	_ = IsGitCLIAvailable()
}

func TestFsck_WithGit(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	// Create a temporary git repository
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	// Initialize a bare repo
	ctx := context.Background()
	if err := initBareRepo(ctx, repoDir); err != nil {
		t.Fatalf("initBareRepo error: %v", err)
	}

	// Run fsck
	if err := Fsck(ctx, repoDir); err != nil {
		t.Errorf("Fsck() error = %v", err)
	}
}

func TestFsck_InvalidRepo(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	tmpDir := t.TempDir()
	ctx := context.Background()

	err := Fsck(ctx, tmpDir)
	if err == nil {
		t.Error("Fsck() should fail on non-git directory")
	}
}

func TestFsck_ContextCancellation(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	ctx := context.Background()
	if err := initBareRepo(ctx, repoDir); err != nil {
		t.Fatalf("initBareRepo error: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Fsck(ctx, repoDir)
	if err == nil {
		t.Error("Fsck() should fail with cancelled context")
	}
}

func TestShellGitClient_Fsck(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	client := NewShellGitClient()
	if client == nil {
		t.Skip("could not create shell git client")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	ctx := context.Background()
	if err := initBareRepo(ctx, repoDir); err != nil {
		t.Fatalf("initBareRepo error: %v", err)
	}

	if err := client.Fsck(ctx, repoDir); err != nil {
		t.Errorf("ShellGitClient.Fsck() error = %v", err)
	}
}

func TestShellGitClient_FsckWithLogging(t *testing.T) {
	if !IsGitInstalled() {
		t.Skip("git not installed")
	}

	var logged bool
	client := NewShellGitClient(
		WithShellLogger(func(msg string, args ...interface{}) {
			logged = true
		}),
	)
	if client == nil {
		t.Skip("could not create shell git client")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	ctx := context.Background()
	if err := initBareRepo(ctx, repoDir); err != nil {
		t.Fatalf("initBareRepo error: %v", err)
	}

	if err := client.Fsck(ctx, repoDir); err != nil {
		t.Errorf("ShellGitClient.Fsck() error = %v", err)
	}

	if !logged {
		t.Error("expected log function to be called")
	}
}

// initBareRepo initializes a bare git repository for testing.
func initBareRepo(_ context.Context, path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "init", "--bare", path)
	return cmd.Run()
}
