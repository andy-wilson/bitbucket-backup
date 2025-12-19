package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewState(t *testing.T) {
	state := NewState("test-workspace")

	if state.Workspace != "test-workspace" {
		t.Errorf("expected workspace 'test-workspace', got '%s'", state.Workspace)
	}
	if state.Version != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", state.Version)
	}
	if state.Projects == nil {
		t.Error("expected projects map to be initialized")
	}
	if state.Repositories == nil {
		t.Error("expected repositories map to be initialized")
	}
}

func TestState_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create and save state
	state := NewState("my-workspace")
	state.UpdateProject("PROJ1", "uuid-1")
	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")
	state.MarkFullBackup()

	if err := state.Save(statePath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load and verify
	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if loaded.Workspace != "my-workspace" {
		t.Errorf("expected workspace 'my-workspace', got '%s'", loaded.Workspace)
	}
	if loaded.LastFullBackup == "" {
		t.Error("expected LastFullBackup to be set")
	}
	if _, ok := loaded.Projects["PROJ1"]; !ok {
		t.Error("expected project PROJ1 to exist")
	}
	if _, ok := loaded.Repositories["repo-1"]; !ok {
		t.Error("expected repository repo-1 to exist")
	}
}

func TestState_LoadNonExistent(t *testing.T) {
	state, err := LoadState("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for nonexistent file")
	}
}

func TestState_HasPreviousBackup(t *testing.T) {
	state := NewState("workspace")

	if state.HasPreviousBackup() {
		t.Error("new state should not have previous backup")
	}

	state.MarkFullBackup()

	if !state.HasPreviousBackup() {
		t.Error("state should have previous backup after marking full")
	}
}

func TestState_MarkBackups(t *testing.T) {
	state := NewState("workspace")

	state.MarkFullBackup()
	if state.LastFullBackup == "" {
		t.Error("LastFullBackup should be set")
	}
	if state.LastIncremental == "" {
		t.Error("LastIncremental should be set after full backup")
	}

	fullTime := state.LastFullBackup

	// Modify the incremental time manually to test the update
	state.LastIncremental = "2020-01-01T00:00:00Z"
	oldIncr := state.LastIncremental

	state.MarkIncrementalBackup()

	if state.LastFullBackup != fullTime {
		t.Error("LastFullBackup should not change on incremental")
	}
	if state.LastIncremental == oldIncr {
		t.Error("LastIncremental should change on incremental")
	}
}

func TestState_UpdateProject(t *testing.T) {
	state := NewState("workspace")

	state.UpdateProject("PROJ1", "uuid-123")

	proj, ok := state.Projects["PROJ1"]
	if !ok {
		t.Fatal("project PROJ1 should exist")
	}
	if proj.UUID != "uuid-123" {
		t.Errorf("expected UUID 'uuid-123', got '%s'", proj.UUID)
	}
	if proj.LastBackedUp == "" {
		t.Error("LastBackedUp should be set")
	}
}

func TestState_UpdateRepository(t *testing.T) {
	state := NewState("workspace")

	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")

	repo, ok := state.Repositories["repo-1"]
	if !ok {
		t.Fatal("repository repo-1 should exist")
	}
	if repo.UUID != "uuid-r1" {
		t.Errorf("expected UUID 'uuid-r1', got '%s'", repo.UUID)
	}
	if repo.ProjectKey != "PROJ1" {
		t.Errorf("expected ProjectKey 'PROJ1', got '%s'", repo.ProjectKey)
	}
	if repo.LastBackedUp == "" {
		t.Error("LastBackedUp should be set")
	}
}

func TestState_PRTimestamps(t *testing.T) {
	state := NewState("workspace")

	// Should return empty for non-existent repo
	if ts := state.GetLastPRUpdated("repo-1"); ts != "" {
		t.Errorf("expected empty timestamp, got '%s'", ts)
	}

	// Add repo
	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")

	// Set PR timestamp
	state.SetRepoLastPRUpdated("repo-1", "2025-01-15T10:00:00Z")

	if ts := state.GetLastPRUpdated("repo-1"); ts != "2025-01-15T10:00:00Z" {
		t.Errorf("expected '2025-01-15T10:00:00Z', got '%s'", ts)
	}
}

func TestState_IssueTimestamps(t *testing.T) {
	state := NewState("workspace")

	// Add repo
	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")

	// Set issue timestamp
	state.SetRepoLastIssueUpdated("repo-1", "2025-01-15T11:00:00Z")

	if ts := state.GetLastIssueUpdated("repo-1"); ts != "2025-01-15T11:00:00Z" {
		t.Errorf("expected '2025-01-15T11:00:00Z', got '%s'", ts)
	}
}

func TestState_IsNewRepo(t *testing.T) {
	state := NewState("workspace")

	if !state.IsNewRepo("repo-1") {
		t.Error("repo-1 should be new")
	}

	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")

	if state.IsNewRepo("repo-1") {
		t.Error("repo-1 should not be new after update")
	}
}

func TestState_GetRepoState(t *testing.T) {
	state := NewState("workspace")

	_, ok := state.GetRepoState("nonexistent")
	if ok {
		t.Error("expected false for nonexistent repo")
	}

	state.UpdateRepository("repo-1", "uuid-r1", "PROJ1")

	repoState, ok := state.GetRepoState("repo-1")
	if !ok {
		t.Error("expected true for existing repo")
	}
	if repoState.UUID != "uuid-r1" {
		t.Errorf("expected UUID 'uuid-r1', got '%s'", repoState.UUID)
	}
}

func TestGetStatePath(t *testing.T) {
	path := GetStatePath("/backups", "my-workspace")
	expected := filepath.Join("/backups", "my-workspace", StateFileName)
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func TestState_SaveCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nested", "dir", "state.json")

	state := NewState("workspace")
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state file should have been created")
	}
}
