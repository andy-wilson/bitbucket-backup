package git

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestNewGoGitClient(t *testing.T) {
	client := NewGoGitClient()
	if client == nil {
		t.Fatal("NewGoGitClient() returned nil")
	}
}

func TestNewGoGitClient_WithOptions(t *testing.T) {
	var logged bool
	logFunc := func(msg string, args ...interface{}) {
		logged = true
	}

	var rateLimited bool
	rateLimitFunc := func() {
		rateLimited = true
	}

	client := NewGoGitClient(
		WithCredentials("user", "pass"),
		WithLogger(logFunc),
		WithProgress(nil),
		WithRateLimit(rateLimitFunc),
		WithSkipSizeCalc(),
	)

	if client == nil {
		t.Fatal("NewGoGitClient() returned nil")
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
	if client.rateLimitFunc == nil {
		t.Error("rateLimitFunc should not be nil")
	}
	if !client.skipSizeCalc {
		t.Error("skipSizeCalc should be true")
	}

	// Verify options work
	client.logFunc("test")
	if !logged {
		t.Error("logFunc was not called")
	}

	client.rateLimitFunc()
	if !rateLimited {
		t.Error("rateLimitFunc was not called")
	}
}

func TestGoGitClient_getAuth(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantNil  bool
	}{
		{
			name:     "with credentials",
			username: "user",
			password: "pass",
			wantNil:  false,
		},
		{
			name:     "no username",
			username: "",
			password: "pass",
			wantNil:  false, // returns auth even with partial creds
		},
		{
			name:     "no password",
			username: "user",
			password: "",
			wantNil:  false, // returns auth even with partial creds
		},
		{
			name:     "no credentials",
			username: "",
			password: "",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewGoGitClient(WithCredentials(tt.username, tt.password))
			auth := client.getAuth()
			if (auth == nil) != tt.wantNil {
				t.Errorf("getAuth() nil = %v, want %v", auth == nil, tt.wantNil)
			}
		})
	}
}

func TestGoGitClient_Fsck(t *testing.T) {
	// Create a temporary directory with a git repo
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")

	// Initialize a bare repo using go-git
	client := NewGoGitClient()

	// First we need to create a valid repo to fsck
	// We'll use initEmptyMirror to create a bare repo
	err := client.initEmptyMirror(repoDir, "https://example.com/test.git")
	if err != nil {
		t.Fatalf("initEmptyMirror error: %v", err)
	}

	// Now run fsck
	ctx := context.Background()
	err = client.Fsck(ctx, repoDir)
	if err != nil {
		t.Errorf("Fsck() error = %v", err)
	}
}

func TestGoGitClient_Fsck_InvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewGoGitClient()
	ctx := context.Background()

	err := client.Fsck(ctx, tmpDir)
	if err == nil {
		t.Error("Fsck() should fail on non-git directory")
	}
}

func TestGoGitClient_initEmptyMirror(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "test-repo")
	repoURL := "https://example.com/test.git"

	client := NewGoGitClient()
	err := client.initEmptyMirror(repoDir, repoURL)
	if err != nil {
		t.Fatalf("initEmptyMirror() error = %v", err)
	}

	// Verify the directory exists
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		t.Error("repository directory was not created")
	}

	// Verify it's a bare git repo (has objects directory)
	objectsDir := filepath.Join(repoDir, "objects")
	if _, err := os.Stat(objectsDir); os.IsNotExist(err) {
		t.Error("objects directory was not created")
	}

	// Verify config exists
	configFile := filepath.Join(repoDir, "config")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestGoGitClient_setupHTTPClient(t *testing.T) {
	client := NewGoGitClient(
		WithRateLimit(func() {}),
	)

	// Setup should be idempotent
	client.setupHTTPClient()
	client.setupHTTPClient()

	if client.httpClient == nil {
		t.Error("httpClient should not be nil after setup")
	}
}

func TestProgressWriter(t *testing.T) {
	var logged string
	logFunc := func(msg string, args ...interface{}) {
		logged = msg
	}

	pw := &progressWriter{logFunc: logFunc}
	n, err := pw.Write([]byte("test progress"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != 13 {
		t.Errorf("Write() n = %d, want 13", n)
	}
	if logged != "  %s" {
		t.Errorf("logged = %q, want '  %%s'", logged)
	}
}

func TestProgressWriter_NilLogFunc(t *testing.T) {
	pw := &progressWriter{logFunc: nil}
	n, err := pw.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != 4 {
		t.Errorf("Write() n = %d, want 4", n)
	}
}

func TestRateLimitedTransport(t *testing.T) {
	var called bool
	transport := &rateLimitedTransport{
		base: http.DefaultTransport,
		rateLimitFunc: func() {
			called = true
		},
	}

	// Create a test request
	req, _ := http.NewRequest("GET", "http://localhost:0/test", nil)

	// The request will fail (nothing listening), but rate limit should be called
	_, _ = transport.RoundTrip(req)
	if !called {
		t.Error("rateLimitFunc was not called")
	}
}

func TestRateLimitedTransport_NilFunc(t *testing.T) {
	transport := &rateLimitedTransport{
		base:          http.DefaultTransport,
		rateLimitFunc: nil,
	}

	// Create a test request
	req, _ := http.NewRequest("GET", "http://localhost:0/test", nil)

	// Should not panic even with nil rateLimitFunc
	_, _ = transport.RoundTrip(req)
}

func TestMaskCredentialsInURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"https://user:pass@bitbucket.org/repo.git",
			"https://%2A%2A%2A@bitbucket.org/repo.git",
		},
		{
			"https://bitbucket.org/repo.git",
			"https://bitbucket.org/repo.git",
		},
	}

	for _, tt := range tests {
		got := maskCredentialsInURL(tt.input)
		if got != tt.want {
			t.Errorf("maskCredentialsInURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

