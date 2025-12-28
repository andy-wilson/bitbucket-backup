package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateFileName is the default state file name.
const StateFileName = ".bb-backup-state.json"

// State tracks the state of previous backups for incremental support.
type State struct {
	Version         string                  `json:"version"`
	Workspace       string                  `json:"workspace"`
	LastFullBackup  string                  `json:"last_full_backup,omitempty"`
	LastIncremental string                  `json:"last_incremental,omitempty"`
	Projects        map[string]ProjectState `json:"projects"`
	Repositories    map[string]RepoState    `json:"repositories"`
	FailedRepos     map[string]FailedRepo   `json:"failed_repos,omitempty"`
}

// FailedRepo tracks a repository that failed to backup.
type FailedRepo struct {
	Slug       string `json:"slug"`
	ProjectKey string `json:"project_key,omitempty"`
	Error      string `json:"error"`
	FailedAt   string `json:"failed_at"`
	Attempts   int    `json:"attempts"`
}

// ProjectState tracks the state of a project.
type ProjectState struct {
	UUID         string `json:"uuid"`
	LastBackedUp string `json:"last_backed_up"`
}

// RepoState tracks the state of a repository.
type RepoState struct {
	UUID             string `json:"uuid"`
	ProjectKey       string `json:"project_key,omitempty"`
	LastCommit       string `json:"last_commit,omitempty"`
	LastPRUpdated    string `json:"last_pr_updated,omitempty"`
	LastIssueUpdated string `json:"last_issue_updated,omitempty"`
	LastBackedUp     string `json:"last_backed_up"`
}

// NewState creates a new empty state.
func NewState(workspace string) *State {
	return &State{
		Version:      "1.0",
		Workspace:    workspace,
		Projects:     make(map[string]ProjectState),
		Repositories: make(map[string]RepoState),
		FailedRepos:  make(map[string]FailedRepo),
	}
}

// LoadState loads state from the given path.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	return &state, nil
}

// Save writes the state to the given path.
func (s *State) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	return nil
}

// MarkFullBackup marks a full backup as completed.
func (s *State) MarkFullBackup() {
	now := time.Now().UTC().Format(time.RFC3339)
	s.LastFullBackup = now
	s.LastIncremental = now
}

// MarkIncrementalBackup marks an incremental backup as completed.
func (s *State) MarkIncrementalBackup() {
	s.LastIncremental = time.Now().UTC().Format(time.RFC3339)
}

// UpdateProject updates the state for a project.
func (s *State) UpdateProject(key, uuid string) {
	s.Projects[key] = ProjectState{
		UUID:         uuid,
		LastBackedUp: time.Now().UTC().Format(time.RFC3339),
	}
}

// UpdateRepository updates the state for a repository.
func (s *State) UpdateRepository(slug, uuid, projectKey string) {
	existing := s.Repositories[slug]
	s.Repositories[slug] = RepoState{
		UUID:             uuid,
		ProjectKey:       projectKey,
		LastCommit:       existing.LastCommit,
		LastPRUpdated:    existing.LastPRUpdated,
		LastIssueUpdated: existing.LastIssueUpdated,
		LastBackedUp:     time.Now().UTC().Format(time.RFC3339),
	}
}

// SetRepoLastPRUpdated sets the last PR updated timestamp for a repo.
func (s *State) SetRepoLastPRUpdated(slug, timestamp string) {
	if repo, ok := s.Repositories[slug]; ok {
		repo.LastPRUpdated = timestamp
		s.Repositories[slug] = repo
	}
}

// SetRepoLastIssueUpdated sets the last issue updated timestamp for a repo.
func (s *State) SetRepoLastIssueUpdated(slug, timestamp string) {
	if repo, ok := s.Repositories[slug]; ok {
		repo.LastIssueUpdated = timestamp
		s.Repositories[slug] = repo
	}
}

// GetRepoState returns the state for a repository.
func (s *State) GetRepoState(slug string) (RepoState, bool) {
	state, ok := s.Repositories[slug]
	return state, ok
}

// GetLastPRUpdated returns the last PR updated timestamp for incremental backup.
func (s *State) GetLastPRUpdated(slug string) string {
	if repo, ok := s.Repositories[slug]; ok {
		return repo.LastPRUpdated
	}
	return ""
}

// GetLastIssueUpdated returns the last issue updated timestamp for incremental backup.
func (s *State) GetLastIssueUpdated(slug string) string {
	if repo, ok := s.Repositories[slug]; ok {
		return repo.LastIssueUpdated
	}
	return ""
}

// HasPreviousBackup returns true if there's a previous backup to build on.
func (s *State) HasPreviousBackup() bool {
	return s.LastFullBackup != "" || s.LastIncremental != ""
}

// IsNewRepo returns true if the repo hasn't been backed up before.
func (s *State) IsNewRepo(slug string) bool {
	_, ok := s.Repositories[slug]
	return !ok
}

// GetStatePath returns the default state file path for a storage path.
func GetStatePath(storagePath, workspace string) string {
	return filepath.Join(storagePath, workspace, StateFileName)
}

// AddFailedRepo records a repository that failed to backup.
func (s *State) AddFailedRepo(slug, projectKey, errMsg string, attempts int) {
	if s.FailedRepos == nil {
		s.FailedRepos = make(map[string]FailedRepo)
	}
	s.FailedRepos[slug] = FailedRepo{
		Slug:       slug,
		ProjectKey: projectKey,
		Error:      errMsg,
		FailedAt:   time.Now().UTC().Format(time.RFC3339),
		Attempts:   attempts,
	}
}

// RemoveFailedRepo removes a repository from the failed list (after successful backup).
func (s *State) RemoveFailedRepo(slug string) {
	if s.FailedRepos != nil {
		delete(s.FailedRepos, slug)
	}
}

// GetFailedRepos returns all failed repositories.
func (s *State) GetFailedRepos() []FailedRepo {
	if s.FailedRepos == nil {
		return nil
	}
	repos := make([]FailedRepo, 0, len(s.FailedRepos))
	for _, r := range s.FailedRepos {
		repos = append(repos, r)
	}
	return repos
}

// HasFailedRepos returns true if there are any failed repositories.
func (s *State) HasFailedRepos() bool {
	return len(s.FailedRepos) > 0
}

// ClearFailedRepos removes all failed repositories from state.
func (s *State) ClearFailedRepos() {
	s.FailedRepos = make(map[string]FailedRepo)
}
