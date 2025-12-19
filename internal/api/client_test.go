package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andy-wilson/bb-backup/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Workspace: "test-workspace",
		Auth: config.AuthConfig{
			Method:      "app_password",
			Username:    "testuser",
			AppPassword: "testpass",
		},
		Storage: config.StorageConfig{
			Type: "local",
			Path: "/backups",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerHour:        36000, // High rate for tests
			BurstSize:              100,
			MaxRetries:             3,
			RetryBackoffSeconds:    1,
			RetryBackoffMultiplier: 2.0,
			MaxBackoffSeconds:      10,
		},
		Parallelism: config.ParallelismConfig{
			GitWorkers: 4,
			APIWorkers: 2,
		},
		Backup: config.BackupConfig{
			IncludePRs:           true,
			IncludePRComments:    true,
			IncludePRActivity:    true,
			IncludeIssues:        true,
			IncludeIssueComments: true,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func TestNewClient(t *testing.T) {
	cfg := testConfig()
	client := NewClient(cfg)

	if client.username != "testuser" {
		t.Errorf("expected username = 'testuser', got '%s'", client.username)
	}
	if client.password != "testpass" {
		t.Errorf("expected password = 'testpass', got '%s'", client.password)
	}
	if client.baseURL != BaseURL {
		t.Errorf("expected baseURL = '%s', got '%s'", BaseURL, client.baseURL)
	}
}

func TestClient_WithOptions(t *testing.T) {
	cfg := testConfig()
	customClient := &http.Client{Timeout: 60 * time.Second}

	client := NewClient(cfg,
		WithHTTPClient(customClient),
		WithBaseURL("https://custom.api.com"),
	)

	if client.httpClient != customClient {
		t.Error("expected custom HTTP client to be set")
	}
	if client.baseURL != "https://custom.api.com" {
		t.Errorf("expected custom baseURL, got '%s'", client.baseURL)
	}
}

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/2.0/test" {
			t.Errorf("expected path /2.0/test, got %s", r.URL.Path)
		}

		// Verify auth header
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
		}
		if user != "testuser" || pass != "testpass" {
			t.Errorf("unexpected credentials: %s:%s", user, pass)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	body, err := client.Get(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status = 'ok', got '%s'", resp["status"])
	}
}

func TestClient_Get_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"type": "error", "error": {"message": "Resource not found"}}`))
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	_, err := client.Get(context.Background(), "/notfound")
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Resource not found" {
		t.Errorf("expected message 'Resource not found', got '%s'", apiErr.Message)
	}
}

func TestClient_Get_RateLimited_WithRetry(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"type": "error", "error": {"message": "Rate limited"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.RateLimit.RetryBackoffSeconds = 1
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	body, err := client.Get(ctx, "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests (2 retries), got %d", requestCount)
	}

	var resp map[string]string
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status = 'ok', got '%s'", resp["status"])
	}
}

func TestClient_Get_RateLimited_MaxRetries(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type": "error", "error": {"message": "Rate limited"}}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.RateLimit.MaxRetries = 2
	cfg.RateLimit.RetryBackoffSeconds = 1
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.Get(ctx, "/test")
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	// Initial request + max retries = 3 total requests
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestClient_GetPaginated(t *testing.T) {
	page := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case 1:
			resp := map[string]interface{}{
				"size":    5,
				"page":    1,
				"pagelen": 2,
				"next":    serverURL + "/2.0/items?page=2",
				"values":  []map[string]string{{"id": "1"}, {"id": "2"}},
			}
			json.NewEncoder(w).Encode(resp)
		case 2:
			resp := map[string]interface{}{
				"size":    5,
				"page":    2,
				"pagelen": 2,
				"next":    serverURL + "/2.0/items?page=3",
				"values":  []map[string]string{{"id": "3"}, {"id": "4"}},
			}
			json.NewEncoder(w).Encode(resp)
		case 3:
			resp := map[string]interface{}{
				"size":    5,
				"page":    3,
				"pagelen": 1,
				"values":  []map[string]string{{"id": "5"}},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	values, err := client.GetPaginated(context.Background(), "/items")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(values) != 5 {
		t.Errorf("expected 5 values, got %d", len(values))
	}

	// Verify the values
	for i, v := range values {
		var item map[string]string
		json.Unmarshal(v, &item)
		expectedID := string(rune('1' + i))
		if item["id"] != expectedID {
			t.Errorf("expected id = '%s', got '%s'", expectedID, item["id"])
		}
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, "/slow")
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		base   string
		params map[string]string
		want   string
	}{
		{
			base:   "/repos",
			params: nil,
			want:   "/repos",
		},
		{
			base:   "/repos",
			params: map[string]string{},
			want:   "/repos",
		},
		{
			base:   "/repos",
			params: map[string]string{"page": "2"},
			want:   "/repos?page=2",
		},
		{
			base:   "/repos",
			params: map[string]string{"page": "2", "pagelen": "50"},
			want:   "/repos?", // Partial match since map order is random
		},
	}

	for _, tt := range tests {
		result := BuildURL(tt.base, tt.params)
		if len(tt.params) > 1 {
			// For multiple params, just verify base is correct
			if result[:len(tt.base)+1] != tt.base+"?" {
				t.Errorf("BuildURL(%s, %v) = %s; want prefix %s?", tt.base, tt.params, result, tt.base)
			}
		} else if result != tt.want {
			t.Errorf("BuildURL(%s, %v) = %s; want %s", tt.base, tt.params, result, tt.want)
		}
	}
}
