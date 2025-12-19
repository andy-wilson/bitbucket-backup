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

// Client is a Bitbucket Cloud API client with built-in rate limiting.
type Client struct {
	httpClient  *http.Client
	baseURL     string
	username    string
	password    string // password, API token, or access token
	rateLimiter *RateLimiter
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
type APIError struct {
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

	for currentURL != "" {
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
	for {
		// Wait for rate limiter
		c.rateLimiter.Wait()

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
		defer resp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		// Handle rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			backoff, shouldRetry := c.rateLimiter.OnRateLimited()
			if !shouldRetry {
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
