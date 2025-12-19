package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVerifyManifest_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := Manifest{
		Workspace: "test-workspace",
		Timestamp: "2025-01-15T10:00:00Z",
		Repositories: []struct {
			Slug    string `json:"slug"`
			Project string `json:"project,omitempty"`
		}{
			{Slug: "repo-1", Project: "PROJ1"},
			{Slug: "repo-2", Project: ""},
		},
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	check := verifyManifest(tmpDir)

	if !check.Exists {
		t.Error("expected manifest to exist")
	}
	if !check.Valid {
		t.Errorf("expected manifest to be valid, got error: %s", check.Error)
	}
	if check.Workspace != "test-workspace" {
		t.Errorf("expected workspace 'test-workspace', got '%s'", check.Workspace)
	}
	if check.RepoCount != 2 {
		t.Errorf("expected repo count 2, got %d", check.RepoCount)
	}
}

func TestVerifyManifest_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	check := verifyManifest(tmpDir)

	if check.Exists {
		t.Error("expected manifest to not exist")
	}
	if check.Valid {
		t.Error("expected manifest to be invalid")
	}
	if check.Error != "manifest.json not found" {
		t.Errorf("expected 'manifest.json not found' error, got '%s'", check.Error)
	}
}

func TestVerifyManifest_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), []byte("not valid json"), 0644)

	check := verifyManifest(tmpDir)

	if !check.Exists {
		t.Error("expected manifest to exist")
	}
	if check.Valid {
		t.Error("expected manifest to be invalid")
	}
	if check.Error == "" {
		t.Error("expected an error message")
	}
}

func TestVerifyJSONFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	data := []byte(`{"name": "test", "value": 123}`)
	filePath := filepath.Join(tmpDir, "test.json")
	os.WriteFile(filePath, data, 0644)

	check := verifyJSONFile(filePath, "test.json")

	if !check.Valid {
		t.Errorf("expected valid JSON, got error: %s", check.Error)
	}
	if check.File != "test.json" {
		t.Errorf("expected file 'test.json', got '%s'", check.File)
	}
}

func TestVerifyJSONFile_Invalid(t *testing.T) {
	tmpDir := t.TempDir()

	data := []byte(`{"name": "test", invalid}`)
	filePath := filepath.Join(tmpDir, "test.json")
	os.WriteFile(filePath, data, 0644)

	check := verifyJSONFile(filePath, "test.json")

	if check.Valid {
		t.Error("expected invalid JSON")
	}
	if check.Error == "" {
		t.Error("expected an error message")
	}
}

func TestVerifyJSONFile_NotFound(t *testing.T) {
	check := verifyJSONFile("/nonexistent/path.json", "path.json")

	if check.Valid {
		t.Error("expected invalid for non-existent file")
	}
	if check.Error != "file not found" {
		t.Errorf("expected 'file not found' error, got '%s'", check.Error)
	}
}

func TestVerifyGitRepo_Valid(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	gitPath := filepath.Join(tmpDir, "repo.git")

	// Create a bare git repository
	cmd := exec.Command("git", "init", "--bare", gitPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create git repo: %v", err)
	}

	check := verifyGitRepo(gitPath)

	if !check.Exists {
		t.Error("expected git repo to exist")
	}
	if !check.Valid {
		t.Errorf("expected git repo to be valid, got error: %s", check.Error)
	}
}

func TestVerifyGitRepo_NotFound(t *testing.T) {
	check := verifyGitRepo("/nonexistent/repo.git")

	if check.Exists {
		t.Error("expected git repo to not exist")
	}
	if check.Valid {
		t.Error("expected git repo to be invalid")
	}
	if check.Error != "repo.git directory not found" {
		t.Errorf("expected 'repo.git directory not found' error, got '%s'", check.Error)
	}
}

func TestVerifyRepository_Complete(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo-1")
	os.MkdirAll(repoPath, 0755)

	// Create git repo
	gitPath := filepath.Join(repoPath, "repo.git")
	exec.Command("git", "init", "--bare", gitPath).Run()

	// Create repository.json
	repoJSON := []byte(`{"slug": "repo-1", "full_name": "workspace/repo-1"}`)
	os.WriteFile(filepath.Join(repoPath, "repository.json"), repoJSON, 0644)

	check := verifyRepository(repoPath, "repo-1", "PROJ1")

	if !check.Valid {
		t.Errorf("expected valid repo, got errors: %v", check.Errors)
	}
	if check.Slug != "repo-1" {
		t.Errorf("expected slug 'repo-1', got '%s'", check.Slug)
	}
	if check.Project != "PROJ1" {
		t.Errorf("expected project 'PROJ1', got '%s'", check.Project)
	}
}

func TestVerifyRepository_MissingGit(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo-1")
	os.MkdirAll(repoPath, 0755)

	// Create repository.json but no git repo
	repoJSON := []byte(`{"slug": "repo-1"}`)
	os.WriteFile(filepath.Join(repoPath, "repository.json"), repoJSON, 0644)

	check := verifyRepository(repoPath, "repo-1", "")

	if check.Valid {
		t.Error("expected invalid repo due to missing git")
	}
	if check.GitCheck == nil {
		t.Error("expected git check to be present")
	}
	if check.GitCheck.Exists {
		t.Error("expected git repo to not exist")
	}
}

func TestVerifyRepository_WithPRsAndIssues(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo-1")
	os.MkdirAll(repoPath, 0755)

	// Create git repo
	gitPath := filepath.Join(repoPath, "repo.git")
	exec.Command("git", "init", "--bare", gitPath).Run()

	// Create repository.json
	os.WriteFile(filepath.Join(repoPath, "repository.json"), []byte(`{}`), 0644)

	// Create PR files
	prDir := filepath.Join(repoPath, "pull-requests")
	os.MkdirAll(filepath.Join(prDir, "1"), 0755)
	os.WriteFile(filepath.Join(prDir, "1.json"), []byte(`{"id": 1}`), 0644)
	os.WriteFile(filepath.Join(prDir, "1", "comments.json"), []byte(`[]`), 0644)
	os.WriteFile(filepath.Join(prDir, "1", "activity.json"), []byte(`[]`), 0644)

	// Create issue files
	issueDir := filepath.Join(repoPath, "issues")
	os.MkdirAll(filepath.Join(issueDir, "1"), 0755)
	os.WriteFile(filepath.Join(issueDir, "1.json"), []byte(`{"id": 1}`), 0644)
	os.WriteFile(filepath.Join(issueDir, "1", "comments.json"), []byte(`[]`), 0644)

	check := verifyRepository(repoPath, "repo-1", "PROJ1")

	if !check.Valid {
		t.Errorf("expected valid repo, got errors: %v", check.Errors)
	}

	// Should have multiple JSON checks
	if len(check.JSONChecks) < 5 {
		t.Errorf("expected at least 5 JSON checks, got %d", len(check.JSONChecks))
	}
}

func TestVerifyRepositoriesFromDirectory(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()

	// Create project structure
	projectRepoPath := filepath.Join(tmpDir, "projects", "PROJ1", "repositories", "repo-1")
	os.MkdirAll(projectRepoPath, 0755)
	exec.Command("git", "init", "--bare", filepath.Join(projectRepoPath, "repo.git")).Run()
	os.WriteFile(filepath.Join(projectRepoPath, "repository.json"), []byte(`{}`), 0644)

	// Create personal repo
	personalRepoPath := filepath.Join(tmpDir, "personal", "repositories", "personal-repo")
	os.MkdirAll(personalRepoPath, 0755)
	exec.Command("git", "init", "--bare", filepath.Join(personalRepoPath, "repo.git")).Run()
	os.WriteFile(filepath.Join(personalRepoPath, "repository.json"), []byte(`{}`), 0644)

	result := &VerifyResult{
		Repositories: make([]RepoCheck, 0),
	}

	verifyRepositoriesFromDirectory(tmpDir, result)

	if len(result.Repositories) != 2 {
		t.Errorf("expected 2 repositories, got %d", len(result.Repositories))
	}

	// Check that we found both repos
	slugs := make(map[string]bool)
	for _, repo := range result.Repositories {
		slugs[repo.Slug] = true
	}

	if !slugs["repo-1"] {
		t.Error("expected to find repo-1")
	}
	if !slugs["personal-repo"] {
		t.Error("expected to find personal-repo")
	}
}
