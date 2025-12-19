package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2.0/repositories/workspace/repo/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"size":    2,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type":     "issue",
					"id":       1,
					"title":    "Bug report",
					"state":    "open",
					"kind":     "bug",
					"priority": "major",
					"reporter": map[string]interface{}{
						"display_name": "Reporter",
					},
				},
				{
					"type":     "issue",
					"id":       2,
					"title":    "Feature request",
					"state":    "open",
					"kind":     "enhancement",
					"priority": "minor",
					"reporter": map[string]interface{}{
						"display_name": "Another Reporter",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	issues, err := client.GetIssues(context.Background(), "workspace", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}

	if issues[0].Title != "Bug report" {
		t.Errorf("expected title 'Bug report', got '%s'", issues[0].Title)
	}

	if issues[0].Kind != "bug" {
		t.Errorf("expected kind 'bug', got '%s'", issues[0].Kind)
	}
}

func TestClient_GetIssues_TrackerDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"type": "error", "error": {"message": "Issue tracker is disabled"}}`))
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	issues, err := client.GetIssues(context.Background(), "workspace", "repo")
	if err != nil {
		t.Fatalf("expected no error for disabled tracker, got: %v", err)
	}

	if len(issues) != 0 {
		t.Errorf("expected empty slice for disabled tracker, got %d issues", len(issues))
	}
}

func TestClient_GetIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2.0/repositories/workspace/repo/issues/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"type":     "issue",
			"id":       42,
			"title":    "Test Issue",
			"state":    "open",
			"kind":     "bug",
			"priority": "critical",
			"content": map[string]interface{}{
				"raw":  "Issue description",
				"html": "<p>Issue description</p>",
			},
			"reporter": map[string]interface{}{
				"display_name": "Reporter",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	issue, err := client.GetIssue(context.Background(), "workspace", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if issue.ID != 42 {
		t.Errorf("expected ID 42, got %d", issue.ID)
	}

	if issue.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got '%s'", issue.Title)
	}

	if issue.Content.Raw != "Issue description" {
		t.Errorf("expected content 'Issue description', got '%s'", issue.Content.Raw)
	}
}

func TestClient_GetIssueComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"size":    2,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type": "issue_comment",
					"id":   1,
					"content": map[string]interface{}{
						"raw": "First comment",
					},
					"user": map[string]interface{}{
						"display_name": "Commenter",
					},
					"created_on": "2025-01-15T10:00:00Z",
				},
				{
					"type": "issue_comment",
					"id":   2,
					"content": map[string]interface{}{
						"raw": "Second comment",
					},
					"user": map[string]interface{}{
						"display_name": "Another Commenter",
					},
					"created_on": "2025-01-15T11:00:00Z",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewClient(cfg, WithBaseURL(server.URL+"/2.0"))

	comments, err := client.GetIssueComments(context.Background(), "workspace", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}

	if comments[0].Content.Raw != "First comment" {
		t.Errorf("expected content 'First comment', got '%s'", comments[0].Content.Raw)
	}
}

func TestClient_GetIssueChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"size":    1,
			"page":    1,
			"pagelen": 10,
			"values": []map[string]interface{}{
				{
					"type":       "issue_change",
					"id":         1,
					"created_on": "2025-01-15T10:00:00Z",
					"user": map[string]interface{}{
						"display_name": "Modifier",
					},
					"changes": map[string]interface{}{
						"state": map[string]interface{}{
							"old": "open",
							"new": "resolved",
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

	changes, err := client.GetIssueChanges(context.Background(), "workspace", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Changes == nil || changes[0].Changes.State == nil {
		t.Error("expected state change")
	} else {
		if changes[0].Changes.State.Old != "open" {
			t.Errorf("expected old state 'open', got '%s'", changes[0].Changes.State.Old)
		}
		if changes[0].Changes.State.New != "resolved" {
			t.Errorf("expected new state 'resolved', got '%s'", changes[0].Changes.State.New)
		}
	}
}
