package git

import (
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
