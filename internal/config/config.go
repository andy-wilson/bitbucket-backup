// Package config handles configuration loading and validation for bb-backup.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration for bb-backup.
type Config struct {
	Workspace   string            `yaml:"workspace"`
	Auth        AuthConfig        `yaml:"auth"`
	Storage     StorageConfig     `yaml:"storage"`
	RateLimit   RateLimitConfig   `yaml:"rate_limit"`
	Parallelism ParallelismConfig `yaml:"parallelism"`
	Backup      BackupConfig      `yaml:"backup"`
	Logging     LoggingConfig     `yaml:"logging"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Method       string `yaml:"method"`
	Username     string `yaml:"username"`
	AppPassword  string `yaml:"app_password"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

// StorageConfig holds storage backend settings.
type StorageConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	RequestsPerHour        int     `yaml:"requests_per_hour"`
	BurstSize              int     `yaml:"burst_size"`
	MaxRetries             int     `yaml:"max_retries"`
	RetryBackoffSeconds    int     `yaml:"retry_backoff_seconds"`
	RetryBackoffMultiplier float64 `yaml:"retry_backoff_multiplier"`
	MaxBackoffSeconds      int     `yaml:"max_backoff_seconds"`
}

// ParallelismConfig holds parallelism settings.
type ParallelismConfig struct {
	GitWorkers int `yaml:"git_workers"`
	APIWorkers int `yaml:"api_workers"`
}

// BackupConfig holds backup content settings.
type BackupConfig struct {
	IncludePRs           bool     `yaml:"include_prs"`
	IncludePRComments    bool     `yaml:"include_pr_comments"`
	IncludePRActivity    bool     `yaml:"include_pr_activity"`
	IncludeIssues        bool     `yaml:"include_issues"`
	IncludeIssueComments bool     `yaml:"include_issue_comments"`
	ExcludeRepos         []string `yaml:"exclude_repos"`
	IncludeRepos         []string `yaml:"include_repos"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// Default returns a Config with sensible default values.
func Default() *Config {
	return &Config{
		Auth: AuthConfig{
			Method: "app_password",
		},
		Storage: StorageConfig{
			Type: "local",
			Path: "./backups",
		},
		RateLimit: RateLimitConfig{
			RequestsPerHour:        900,
			BurstSize:              10,
			MaxRetries:             5,
			RetryBackoffSeconds:    5,
			RetryBackoffMultiplier: 2.0,
			MaxBackoffSeconds:      300,
		},
		Parallelism: ParallelismConfig{
			GitWorkers: 4,
			APIWorkers: 2,
		},
		Backup: BackupConfig{
			IncludePRs:           true,
			IncludePRComments:    true,
			IncludePRActivity:    true,
			IncludeIssues:        true,
			IncludeIssueComments: true,
			ExcludeRepos:         []string{},
			IncludeRepos:         []string{},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load reads a configuration file and returns a Config.
// Environment variables in the format ${VAR_NAME} are substituted.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from YAML bytes.
// Environment variables in the format ${VAR_NAME} are substituted.
func Parse(data []byte) (*Config, error) {
	// Substitute environment variables before parsing
	expanded := expandEnvVars(string(data))

	cfg := Default()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// envVarRegex matches ${VAR_NAME} patterns.
var envVarRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// expandEnvVars replaces ${VAR_NAME} with the value of the environment variable.
// If the variable is not set, it is replaced with an empty string.
func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := match[2 : len(match)-1]
		return os.Getenv(varName)
	})
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	if c.Workspace == "" {
		errs = append(errs, "workspace is required")
	}

	// Validate auth
	switch c.Auth.Method {
	case "app_password":
		if c.Auth.Username == "" {
			errs = append(errs, "auth.username is required for app_password method")
		}
		if c.Auth.AppPassword == "" {
			errs = append(errs, "auth.app_password is required for app_password method")
		}
	case "oauth":
		if c.Auth.ClientID == "" {
			errs = append(errs, "auth.client_id is required for oauth method")
		}
		if c.Auth.ClientSecret == "" {
			errs = append(errs, "auth.client_secret is required for oauth method")
		}
	case "":
		errs = append(errs, "auth.method is required")
	default:
		errs = append(errs, fmt.Sprintf("auth.method must be 'app_password' or 'oauth', got '%s'", c.Auth.Method))
	}

	// Validate storage
	switch c.Storage.Type {
	case "local":
		if c.Storage.Path == "" {
			errs = append(errs, "storage.path is required for local storage")
		}
	case "":
		errs = append(errs, "storage.type is required")
	default:
		errs = append(errs, fmt.Sprintf("storage.type must be 'local', got '%s'", c.Storage.Type))
	}

	// Validate rate limit
	if c.RateLimit.RequestsPerHour <= 0 {
		errs = append(errs, "rate_limit.requests_per_hour must be positive")
	}
	if c.RateLimit.BurstSize <= 0 {
		errs = append(errs, "rate_limit.burst_size must be positive")
	}
	if c.RateLimit.MaxRetries < 0 {
		errs = append(errs, "rate_limit.max_retries must be non-negative")
	}

	// Validate parallelism
	if c.Parallelism.GitWorkers <= 0 {
		errs = append(errs, "parallelism.git_workers must be positive")
	}
	if c.Parallelism.APIWorkers <= 0 {
		errs = append(errs, "parallelism.api_workers must be positive")
	}

	// Validate logging
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("logging.level must be debug/info/warn/error, got '%s'", c.Logging.Level))
	}

	switch c.Logging.Format {
	case "text", "json":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("logging.format must be text/json, got '%s'", c.Logging.Format))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
