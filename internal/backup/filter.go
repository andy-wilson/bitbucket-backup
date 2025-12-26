package backup

import (
	"path/filepath"

	"github.com/andy-wilson/bb-backup/internal/api"
)

// LogFunc is called to log debug messages.
type LogFunc func(msg string, args ...interface{})

// RepoFilter filters repositories based on include/exclude patterns.
type RepoFilter struct {
	includePatterns []string
	excludePatterns []string
	logFunc         LogFunc
}

// NewRepoFilter creates a new repository filter.
func NewRepoFilter(include, exclude []string) *RepoFilter {
	return &RepoFilter{
		includePatterns: include,
		excludePatterns: exclude,
	}
}

// NewRepoFilterWithLog creates a new repository filter with logging.
func NewRepoFilterWithLog(include, exclude []string, logFunc LogFunc) *RepoFilter {
	return &RepoFilter{
		includePatterns: include,
		excludePatterns: exclude,
		logFunc:         logFunc,
	}
}

// Filter returns repositories that pass the filter criteria.
func (f *RepoFilter) Filter(repos []api.Repository) []api.Repository {
	if len(f.includePatterns) == 0 && len(f.excludePatterns) == 0 {
		return repos
	}

	var filtered []api.Repository
	for _, repo := range repos {
		included, reason := f.shouldIncludeWithReason(repo.Slug)
		if included {
			filtered = append(filtered, repo)
		} else if f.logFunc != nil {
			f.logFunc("Filter excluded: %s (%s)", repo.Slug, reason)
		}
	}
	return filtered
}

// ShouldInclude checks if a repository should be included in the backup.
func (f *RepoFilter) ShouldInclude(repoSlug string) bool {
	included, _ := f.shouldIncludeWithReason(repoSlug)
	return included
}

// shouldIncludeWithReason checks if a repository should be included and returns the reason.
func (f *RepoFilter) shouldIncludeWithReason(repoSlug string) (bool, string) {
	// First check exclusions
	for _, pattern := range f.excludePatterns {
		if matched, _ := filepath.Match(pattern, repoSlug); matched {
			return false, "matched exclude pattern \"" + pattern + "\""
		}
	}

	// If no include patterns, include by default (minus exclusions)
	if len(f.includePatterns) == 0 {
		return true, ""
	}

	// Check include patterns
	for _, pattern := range f.includePatterns {
		if matched, _ := filepath.Match(pattern, repoSlug); matched {
			return true, ""
		}
	}

	// Didn't match any include pattern
	return false, "did not match any include pattern"
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

// SingleRepoSlug returns the repo slug if the filter specifies exactly one
// specific repository (no wildcards), and an empty string otherwise.
// This is used to optimize single-repo backups by fetching directly from the API.
func (f *RepoFilter) SingleRepoSlug() string {
	// Must have exactly one include pattern and no exclude patterns
	if len(f.includePatterns) != 1 || len(f.excludePatterns) > 0 {
		return ""
	}

	pattern := f.includePatterns[0]

	// Check if pattern contains any glob metacharacters
	for _, c := range pattern {
		if c == '*' || c == '?' || c == '[' || c == '\\' {
			return ""
		}
	}

	return pattern
}
