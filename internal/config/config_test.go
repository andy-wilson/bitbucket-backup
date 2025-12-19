package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Auth.Method != "app_password" {
		t.Errorf("expected auth.method = 'app_password', got '%s'", cfg.Auth.Method)
	}
	if cfg.Storage.Type != "local" {
		t.Errorf("expected storage.type = 'local', got '%s'", cfg.Storage.Type)
	}
	if cfg.RateLimit.RequestsPerHour != 900 {
		t.Errorf("expected rate_limit.requests_per_hour = 900, got %d", cfg.RateLimit.RequestsPerHour)
	}
	if cfg.Parallelism.GitWorkers != 4 {
		t.Errorf("expected parallelism.git_workers = 4, got %d", cfg.Parallelism.GitWorkers)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected logging.level = 'info', got '%s'", cfg.Logging.Level)
	}
}

func TestParse_ValidConfig(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "testuser"
  app_password: "testpass"
storage:
  type: "local"
  path: "/backups"
rate_limit:
  requests_per_hour: 800
  burst_size: 5
  max_retries: 3
  retry_backoff_seconds: 10
  retry_backoff_multiplier: 1.5
  max_backoff_seconds: 120
parallelism:
  git_workers: 2
  api_workers: 1
backup:
  include_prs: true
  include_issues: false
logging:
  level: "debug"
  format: "json"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Workspace != "my-workspace" {
		t.Errorf("expected workspace = 'my-workspace', got '%s'", cfg.Workspace)
	}
	if cfg.Auth.Username != "testuser" {
		t.Errorf("expected auth.username = 'testuser', got '%s'", cfg.Auth.Username)
	}
	if cfg.RateLimit.RequestsPerHour != 800 {
		t.Errorf("expected rate_limit.requests_per_hour = 800, got %d", cfg.RateLimit.RequestsPerHour)
	}
	if cfg.Parallelism.GitWorkers != 2 {
		t.Errorf("expected parallelism.git_workers = 2, got %d", cfg.Parallelism.GitWorkers)
	}
	if cfg.Backup.IncludeIssues != false {
		t.Errorf("expected backup.include_issues = false, got %v", cfg.Backup.IncludeIssues)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level = 'debug', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected logging.format = 'json', got '%s'", cfg.Logging.Format)
	}
}

func TestParse_EnvVarSubstitution(t *testing.T) {
	// Set environment variables for the test
	os.Setenv("TEST_BB_USERNAME", "env-user")
	os.Setenv("TEST_BB_PASSWORD", "env-pass")
	defer os.Unsetenv("TEST_BB_USERNAME")
	defer os.Unsetenv("TEST_BB_PASSWORD")

	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "${TEST_BB_USERNAME}"
  app_password: "${TEST_BB_PASSWORD}"
storage:
  type: "local"
  path: "/backups"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Auth.Username != "env-user" {
		t.Errorf("expected auth.username = 'env-user', got '%s'", cfg.Auth.Username)
	}
	if cfg.Auth.AppPassword != "env-pass" {
		t.Errorf("expected auth.app_password = 'env-pass', got '%s'", cfg.Auth.AppPassword)
	}
}

func TestParse_UnsetEnvVar(t *testing.T) {
	// Ensure the env var is not set
	os.Unsetenv("UNSET_VAR_FOR_TEST")

	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "${UNSET_VAR_FOR_TEST}"
  app_password: "somepass"
storage:
  type: "local"
  path: "/backups"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected validation error for empty username")
	}
}

func TestParse_MissingWorkspace(t *testing.T) {
	yaml := `
auth:
  method: "app_password"
  username: "user"
  app_password: "pass"
storage:
  type: "local"
  path: "/backups"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
}

func TestParse_InvalidAuthMethod(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "invalid"
  username: "user"
  app_password: "pass"
storage:
  type: "local"
  path: "/backups"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid auth method")
	}
}

func TestParse_OAuthMethod(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "oauth"
  client_id: "my-client-id"
  client_secret: "my-client-secret"
storage:
  type: "local"
  path: "/backups"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Auth.Method != "oauth" {
		t.Errorf("expected auth.method = 'oauth', got '%s'", cfg.Auth.Method)
	}
	if cfg.Auth.ClientID != "my-client-id" {
		t.Errorf("expected auth.client_id = 'my-client-id', got '%s'", cfg.Auth.ClientID)
	}
}

func TestParse_InvalidStorageType(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "user"
  app_password: "pass"
storage:
  type: "s3"
  path: "/backups"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unsupported storage type")
	}
}

func TestParse_InvalidLogLevel(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "user"
  app_password: "pass"
storage:
  type: "local"
  path: "/backups"
logging:
  level: "trace"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `
workspace: [invalid
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
workspace: "test-workspace"
auth:
  method: "app_password"
  username: "fileuser"
  app_password: "filepass"
storage:
  type: "local"
  path: "/backups"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Workspace != "test-workspace" {
		t.Errorf("expected workspace = 'test-workspace', got '%s'", cfg.Workspace)
	}
	if cfg.Auth.Username != "fileuser" {
		t.Errorf("expected auth.username = 'fileuser', got '%s'", cfg.Auth.Username)
	}
}

func TestValidate_NegativeRateLimit(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "user"
  app_password: "pass"
storage:
  type: "local"
  path: "/backups"
rate_limit:
  requests_per_hour: -1
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for negative rate limit")
	}
}

func TestValidate_ZeroWorkers(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "app_password"
  username: "user"
  app_password: "pass"
storage:
  type: "local"
  path: "/backups"
parallelism:
  git_workers: 0
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for zero git workers")
	}
}

func TestParse_APITokenMethod(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "api_token"
  username: "myuser"
  email: "myuser@example.com"
  api_token: "my-api-token"
storage:
  type: "local"
  path: "/backups"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Auth.Method != "api_token" {
		t.Errorf("expected auth.method = 'api_token', got '%s'", cfg.Auth.Method)
	}
	if cfg.Auth.Username != "myuser" {
		t.Errorf("expected auth.username = 'myuser', got '%s'", cfg.Auth.Username)
	}
	if cfg.Auth.Email != "myuser@example.com" {
		t.Errorf("expected auth.email = 'myuser@example.com', got '%s'", cfg.Auth.Email)
	}
	if cfg.Auth.APIToken != "my-api-token" {
		t.Errorf("expected auth.api_token = 'my-api-token', got '%s'", cfg.Auth.APIToken)
	}
}

func TestParse_APITokenMethod_MissingEmail(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "api_token"
  username: "myuser"
  api_token: "my-api-token"
storage:
  type: "local"
  path: "/backups"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing email with api_token method")
	}
}

func TestParse_AccessTokenMethod(t *testing.T) {
	yaml := `
workspace: "my-workspace"
auth:
  method: "access_token"
  access_token: "repo-access-token"
storage:
  type: "local"
  path: "/backups"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Auth.Method != "access_token" {
		t.Errorf("expected auth.method = 'access_token', got '%s'", cfg.Auth.Method)
	}
	if cfg.Auth.AccessToken != "repo-access-token" {
		t.Errorf("expected auth.access_token = 'repo-access-token', got '%s'", cfg.Auth.AccessToken)
	}
}

func TestGetAPICredentials_AppPassword(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "app_password"
	cfg.Auth.Username = "user"
	cfg.Auth.AppPassword = "pass"

	username, password := cfg.GetAPICredentials()
	if username != "user" {
		t.Errorf("expected username = 'user', got '%s'", username)
	}
	if password != "pass" {
		t.Errorf("expected password = 'pass', got '%s'", password)
	}
}

func TestGetAPICredentials_APIToken(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "api_token"
	cfg.Auth.Username = "user"
	cfg.Auth.Email = "user@example.com"
	cfg.Auth.APIToken = "token123"

	username, password := cfg.GetAPICredentials()
	if username != "user" {
		t.Errorf("expected username = 'user', got '%s'", username)
	}
	if password != "token123" {
		t.Errorf("expected password = 'token123', got '%s'", password)
	}
}

func TestGetAPICredentials_AccessToken(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "access_token"
	cfg.Auth.AccessToken = "repo-token"

	username, password := cfg.GetAPICredentials()
	if username != "x-token-auth" {
		t.Errorf("expected username = 'x-token-auth', got '%s'", username)
	}
	if password != "repo-token" {
		t.Errorf("expected password = 'repo-token', got '%s'", password)
	}
}

func TestGetGitCredentials_AppPassword(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "app_password"
	cfg.Auth.Username = "user"
	cfg.Auth.AppPassword = "pass"

	username, password := cfg.GetGitCredentials()
	if username != "user" {
		t.Errorf("expected username = 'user', got '%s'", username)
	}
	if password != "pass" {
		t.Errorf("expected password = 'pass', got '%s'", password)
	}
}

func TestGetGitCredentials_APIToken(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "api_token"
	cfg.Auth.Username = "user"
	cfg.Auth.Email = "user@example.com"
	cfg.Auth.APIToken = "token123"

	username, password := cfg.GetGitCredentials()
	// API tokens require email for git operations
	if username != "user@example.com" {
		t.Errorf("expected username = 'user@example.com', got '%s'", username)
	}
	if password != "token123" {
		t.Errorf("expected password = 'token123', got '%s'", password)
	}
}

func TestGetGitCredentials_AccessToken(t *testing.T) {
	cfg := Default()
	cfg.Workspace = "test"
	cfg.Auth.Method = "access_token"
	cfg.Auth.AccessToken = "repo-token"

	username, password := cfg.GetGitCredentials()
	if username != "x-token-auth" {
		t.Errorf("expected username = 'x-token-auth', got '%s'", username)
	}
	if password != "repo-token" {
		t.Errorf("expected password = 'repo-token', got '%s'", password)
	}
}
