package backup

import (
	"testing"

	"github.com/andy-wilson/bb-backup/internal/api"
)

func TestRepoFilter_NoPatterns(t *testing.T) {
	filter := NewRepoFilter(nil, nil)

	repos := []api.Repository{
		{Slug: "repo-1"},
		{Slug: "repo-2"},
		{Slug: "repo-3"},
	}

	filtered := filter.Filter(repos)
	if len(filtered) != 3 {
		t.Errorf("expected 3 repos, got %d", len(filtered))
	}
}

func TestRepoFilter_IncludeOnly(t *testing.T) {
	filter := NewRepoFilter([]string{"core-*", "platform-*"}, nil)

	repos := []api.Repository{
		{Slug: "core-api"},
		{Slug: "core-web"},
		{Slug: "platform-auth"},
		{Slug: "test-repo"},
		{Slug: "random"},
	}

	filtered := filter.Filter(repos)
	if len(filtered) != 3 {
		t.Errorf("expected 3 repos, got %d", len(filtered))
	}

	// Verify the right ones were included
	slugs := make(map[string]bool)
	for _, r := range filtered {
		slugs[r.Slug] = true
	}

	if !slugs["core-api"] {
		t.Error("expected core-api to be included")
	}
	if !slugs["core-web"] {
		t.Error("expected core-web to be included")
	}
	if !slugs["platform-auth"] {
		t.Error("expected platform-auth to be included")
	}
	if slugs["test-repo"] {
		t.Error("expected test-repo to be excluded")
	}
}

func TestRepoFilter_ExcludeOnly(t *testing.T) {
	filter := NewRepoFilter(nil, []string{"test-*", "archive-*"})

	repos := []api.Repository{
		{Slug: "core-api"},
		{Slug: "test-unit"},
		{Slug: "test-integration"},
		{Slug: "archive-old"},
		{Slug: "production"},
	}

	filtered := filter.Filter(repos)
	if len(filtered) != 2 {
		t.Errorf("expected 2 repos, got %d", len(filtered))
	}

	slugs := make(map[string]bool)
	for _, r := range filtered {
		slugs[r.Slug] = true
	}

	if !slugs["core-api"] {
		t.Error("expected core-api to be included")
	}
	if !slugs["production"] {
		t.Error("expected production to be included")
	}
	if slugs["test-unit"] {
		t.Error("expected test-unit to be excluded")
	}
}

func TestRepoFilter_IncludeAndExclude(t *testing.T) {
	// Include all api-* repos, but exclude api-test-*
	filter := NewRepoFilter([]string{"api-*"}, []string{"api-test-*"})

	repos := []api.Repository{
		{Slug: "api-users"},
		{Slug: "api-orders"},
		{Slug: "api-test-mock"},
		{Slug: "api-test-fixtures"},
		{Slug: "web-frontend"},
	}

	filtered := filter.Filter(repos)
	if len(filtered) != 2 {
		t.Errorf("expected 2 repos, got %d", len(filtered))
	}

	slugs := make(map[string]bool)
	for _, r := range filtered {
		slugs[r.Slug] = true
	}

	if !slugs["api-users"] {
		t.Error("expected api-users to be included")
	}
	if !slugs["api-orders"] {
		t.Error("expected api-orders to be included")
	}
	if slugs["api-test-mock"] {
		t.Error("expected api-test-mock to be excluded")
	}
}

func TestRepoFilter_ShouldInclude(t *testing.T) {
	filter := NewRepoFilter([]string{"include-*"}, []string{"*-exclude"})

	tests := []struct {
		slug     string
		expected bool
	}{
		{"include-this", true},
		{"include-exclude", false}, // Exclusion takes precedence
		{"random-repo", false},
		{"include-", true},
	}

	for _, tt := range tests {
		result := filter.ShouldInclude(tt.slug)
		if result != tt.expected {
			t.Errorf("ShouldInclude(%s) = %v, expected %v", tt.slug, result, tt.expected)
		}
	}
}

func TestRepoFilter_FilteredCount(t *testing.T) {
	filter := NewRepoFilter([]string{"keep-*"}, nil)

	repos := []api.Repository{
		{Slug: "keep-1"},
		{Slug: "keep-2"},
		{Slug: "drop-1"},
		{Slug: "drop-2"},
		{Slug: "drop-3"},
	}

	included, excluded := filter.FilteredCount(repos)
	if included != 2 {
		t.Errorf("expected 2 included, got %d", included)
	}
	if excluded != 3 {
		t.Errorf("expected 3 excluded, got %d", excluded)
	}
}
