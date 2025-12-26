package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/andy-wilson/bb-backup/internal/config"
)

const (
	// BaseURL is the Bitbucket Cloud API v2 base URL.
	BaseURL = "https://api.bitbucket.org/2.0"

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second
)

// ProgressFunc is called during pagination to report progress.
// It receives the current page number and total items fetched so far.
type ProgressFunc func(page int, itemsSoFar int)

// LogFunc is called to log debug messages.
type LogFunc func(msg string, args ...interface{})

// Client is a Bitbucket Cloud API client with built-in rate limiting.
type Client struct {
	httpClient   *http.Client
	baseURL      string
	username     string
	password     string // password, API token, or access token
	rateLimiter  *RateLimiter
	progressFunc ProgressFunc
	logFunc      LogFunc
}

// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithBaseURL sets a custom base URL (useful for testing).
func WithBaseURL(url string) ClientOption {
	return func(client *Client) {
		client.baseURL = url
	}
}

// WithProgressFunc sets a callback for pagination progress.
func WithProgressFunc(f ProgressFunc) ClientOption {
	return func(client *Client) {
		client.progressFunc = f
	}
}

// WithLogFunc sets a callback for debug logging.
func WithLogFunc(f LogFunc) ClientOption {
	return func(client *Client) {
		client.logFunc = f
	}
}

// NewClient creates a new Bitbucket API client from configuration.
func NewClient(cfg *config.Config, opts ...ClientOption) *Client {
	rlConfig := RateLimiterConfig{
		RequestsPerHour:        cfg.RateLimit.RequestsPerHour,
		BurstSize:              cfg.RateLimit.BurstSize,
		MaxRetries:             cfg.RateLimit.MaxRetries,
		RetryBackoffSeconds:    cfg.RateLimit.RetryBackoffSeconds,
		RetryBackoffMultiplier: cfg.RateLimit.RetryBackoffMultiplier,
		MaxBackoffSeconds:      cfg.RateLimit.MaxBackoffSeconds,
	}

	// Get the appropriate credentials for API calls
	username, password := cfg.GetAPICredentials()

	c := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL:     BaseURL,
		username:    username,
		password:    password,
		rateLimiter: NewRateLimiter(rlConfig),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// RateLimiter returns the rate limiter for this client.
// This allows other components to share the same rate limiting.
func (c *Client) RateLimiter() *RateLimiter {
	return c.rateLimiter
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Size     int             `json:"size"`
	Page     int             `json:"page"`
	PageLen  int             `json:"pagelen"`
	Next     string          `json:"next"`
	Previous string          `json:"previous"`
	Values   json.RawMessage `json:"values"`
}

// Error represents a Bitbucket API error response.
type Error struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// APIError is returned when the API returns an error response.
type APIError struct { //nolint:revive // intentional naming for clarity
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bitbucket API error (status %d): %s", e.StatusCode, e.Message)
}

// Get performs a GET request to the given path.
// The path should be relative to the API base URL (e.g., "/workspaces/myworkspace").
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// GetPaginated fetches all pages of a paginated endpoint and returns all values.
func (c *Client) GetPaginated(ctx context.Context, path string) ([]json.RawMessage, error) {
	var allValues []json.RawMessage
	currentURL := c.baseURL + path
	page := 0

	for currentURL != "" {
		page++

		body, err := c.doURL(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing paginated response: %w", err)
		}

		// Parse the values array
		var values []json.RawMessage
		if err := json.Unmarshal(resp.Values, &values); err != nil {
			return nil, fmt.Errorf("parsing values array: %w", err)
		}

		allValues = append(allValues, values...)

		// Report progress if callback is set
		if c.progressFunc != nil {
			c.progressFunc(page, len(allValues))
		}

		currentURL = resp.Next
	}

	return allValues, nil
}

// do performs an HTTP request with rate limiting and retry logic.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	fullURL := c.baseURL + path
	return c.doURL(ctx, method, fullURL, body)
}

// doURL performs an HTTP request to an absolute URL.
func (c *Client) doURL(ctx context.Context, method, fullURL string, body io.Reader) ([]byte, error) {
	attempt := 0
	for {
		attempt++

		// Wait for rate limiter
		c.rateLimiter.Wait()

		// Log the request
		if c.logFunc != nil {
			c.logFunc("API %s %s", method, fullURL)
		}

		startTime := time.Now()

		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		// Set authentication
		req.SetBasicAuth(c.username, c.password)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}
		defer resp.Body.Close() //nolint:errcheck // closing response body

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		elapsed := time.Since(startTime)

		// Log response details
		if c.logFunc != nil {
			c.logFunc("  → %d %s (took %s, %s)",
				resp.StatusCode, http.StatusText(resp.StatusCode),
				elapsed.Round(time.Millisecond), formatBytes(len(respBody)))

			// Log rate limit headers if present
			if limit := resp.Header.Get("X-RateLimit-Limit"); limit != "" {
				remaining := resp.Header.Get("X-RateLimit-Remaining")
				reset := resp.Header.Get("X-RateLimit-Reset")
				c.logFunc("  Rate limit: %s/%s remaining (resets: %s)", remaining, limit, reset)
			}
		}

		// Handle rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			backoff, shouldRetry := c.rateLimiter.OnRateLimited()
			if !shouldRetry {
				if c.logFunc != nil {
					c.logFunc("  Rate limited: max retries (%d) reached, giving up", attempt)
				}
				return nil, &APIError{
					StatusCode: resp.StatusCode,
					Message:    "rate limit exceeded, max retries reached",
				}
			}

			// Check for Retry-After header
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					backoff = time.Duration(seconds) * time.Second
				}
			}

			if c.logFunc != nil {
				c.logFunc("  Rate limited: retry %d after %s backoff", attempt, backoff.Round(time.Second))
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		// Handle other errors
		if resp.StatusCode >= 400 {
			var apiErr Error
			if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Message != "" {
				return nil, &APIError{
					StatusCode: resp.StatusCode,
					Message:    apiErr.Error.Message,
				}
			}
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Message:    string(respBody),
			}
		}

		// Success
		c.rateLimiter.OnSuccess()
		return respBody, nil
	}
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := unit, 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMG"[exp])
}

// BuildURL constructs a URL with query parameters.
func BuildURL(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return base + "?" + values.Encode()
}
