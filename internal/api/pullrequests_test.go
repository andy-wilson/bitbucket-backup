package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetPullRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2.0/repositories/workspace/repo/pullrequests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"size":    2,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type":  "pullrequest",
					"id":    1,
					"title": "First PR",
					"state": "OPEN",
					"author": map[string]interface{}{
						"type":         "user",
						"display_name": "Test User",
					},
				},
				{
					"type":  "pullrequest",
					"id":    2,
					"title": "Second PR",
					"state": "MERGED",
					"author": map[string]interface{}{
						"type":         "user",
						"display_name": "Another User",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	prs, err := client.GetPullRequests(context.Background(), "workspace", "repo", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 2 {
		t.Errorf("expected 2 PRs, got %d", len(prs))
	}

	if prs[0].Title != "First PR" {
		t.Errorf("expected title 'First PR', got '%s'", prs[0].Title)
	}

	if prs[1].State != "MERGED" {
		t.Errorf("expected state 'MERGED', got '%s'", prs[1].State)
	}
}

func TestClient_GetPullRequestsWithState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != "OPEN" {
			t.Errorf("expected state=OPEN, got %s", state)
		}

		resp := map[string]interface{}{
			"size":    1,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type":  "pullrequest",
					"id":    1,
					"title": "Open PR",
					"state": "OPEN",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	prs, err := client.GetPullRequests(context.Background(), "workspace", "repo", "OPEN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 1 {
		t.Errorf("expected 1 PR, got %d", len(prs))
	}
}

func TestClient_GetPullRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2.0/repositories/workspace/repo/pullrequests/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"type":        "pullrequest",
			"id":          42,
			"title":       "Test PR",
			"description": "A test pull request",
			"state":       "OPEN",
			"author": map[string]interface{}{
				"type":         "user",
				"display_name": "Test User",
			},
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	pr, err := client.GetPullRequest(context.Background(), "workspace", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.ID != 42 {
		t.Errorf("expected ID 42, got %d", pr.ID)
	}

	if pr.Title != "Test PR" {
		t.Errorf("expected title 'Test PR', got '%s'", pr.Title)
	}

	if pr.Source.Branch.Name != "feature-branch" {
		t.Errorf("expected source branch 'feature-branch', got '%s'", pr.Source.Branch.Name)
	}
}

func TestClient_GetPullRequestComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"size":    2,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type": "pullrequest_comment",
					"id":   1,
					"content": map[string]interface{}{
						"raw": "Great work!",
					},
					"user": map[string]interface{}{
						"display_name": "Reviewer",
					},
				},
				{
					"type": "pullrequest_comment",
					"id":   2,
					"content": map[string]interface{}{
						"raw": "Inline comment",
					},
					"inline": map[string]interface{}{
						"path": "src/main.go",
						"to":   42,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	comments, err := client.GetPullRequestComments(context.Background(), "workspace", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}

	if comments[0].Content.Raw != "Great work!" {
		t.Errorf("expected content 'Great work!', got '%s'", comments[0].Content.Raw)
	}

	if comments[1].Inline == nil {
		t.Error("expected inline comment to have inline data")
	} else if comments[1].Inline.Path != "src/main.go" {
		t.Errorf("expected inline path 'src/main.go', got '%s'", comments[1].Inline.Path)
	}
}

func TestClient_GetPullRequestActivity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"size":    2,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"approval": map[string]interface{}{
						"date": "2025-01-15T10:00:00Z",
						"user": map[string]interface{}{
							"display_name": "Approver",
						},
					},
				},
				{
					"update": map[string]interface{}{
						"date":  "2025-01-15T09:00:00Z",
						"state": "OPEN",
						"author": map[string]interface{}{
							"display_name": "Author",
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	activities, err := client.GetPullRequestActivity(context.Background(), "workspace", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(activities) != 2 {
		t.Errorf("expected 2 activities, got %d", len(activities))
	}

	if activities[0].Approval == nil {
		t.Error("expected first activity to be an approval")
	}

	if activities[1].Update == nil {
		t.Error("expected second activity to be an update")
	}
}
