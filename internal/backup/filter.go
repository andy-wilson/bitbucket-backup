package backup

import (
	"path/filepath"

	"github.com/andy-wilson/bb-backup/internal/api"
)

// RepoFilter filters repositories based on include/exclude patterns.
type RepoFilter struct {
	includePatterns []string
	excludePatterns []string
}

// NewRepoFilter creates a new repository filter.
func NewRepoFilter(include, exclude []string) *RepoFilter {
	return &RepoFilter{
		includePatterns: include,
		excludePatterns: exclude,
	}
}

// Filter returns repositories that pass the filter criteria.
func (f *RepoFilter) Filter(repos []api.Repository) []api.Repository {
	if len(f.includePatterns) == 0 && len(f.excludePatterns) == 0 {
		return repos
	}

	var filtered []api.Repository
	for _, repo := range repos {
		if f.ShouldInclude(repo.Slug) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

// ShouldInclude checks if a repository should be included in the backup.
func (f *RepoFilter) ShouldInclude(repoSlug string) bool {
	// First check exclusions
	for _, pattern := range f.excludePatterns {
		if matched, _ := filepath.Match(pattern, repoSlug); matched {
			return false
		}
	}

	// If no include patterns, include by default (minus exclusions)
	if len(f.includePatterns) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range f.includePatterns {
		if matched, _ := filepath.Match(pattern, repoSlug); matched {
			return true
		}
	}

	// Didn't match any include pattern
	return false
}

// FilteredCount returns counts of included and excluded repos.
func (f *RepoFilter) FilteredCount(repos []api.Repository) (included, excluded int) {
	for _, repo := range repos {
		if f.ShouldInclude(repo.Slug) {
			included++
		} else {
			excluded++
		}
	}
	return
}
