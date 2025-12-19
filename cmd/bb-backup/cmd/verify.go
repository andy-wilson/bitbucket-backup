package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	verifyJSON    bool
	verifyVerbose bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify [backup-path]",
	Short: "Verify backup integrity",
	Long: `Verify the integrity of a backup.

This command checks:
  - Manifest file exists and is valid JSON
  - All referenced repositories exist
  - Git repositories pass fsck checks
  - All metadata JSON files are valid

Exit codes:
  0 - All checks passed
  1 - One or more checks failed

Examples:
  bb-backup verify /backups/my-workspace
  bb-backup verify /backups/my-workspace --json
  bb-backup verify /backups/my-workspace -v`,
	Args: cobra.ExactArgs(1),
	RunE: runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)

	verifyCmd.Flags().BoolVar(&verifyJSON, "json", false, "output results as JSON")
	verifyCmd.Flags().BoolVarP(&verifyVerbose, "verbose", "v", false, "show detailed output")
}

// VerifyResult represents the result of verification.
type VerifyResult struct {
	Path         string         `json:"path"`
	Valid        bool           `json:"valid"`
	Manifest     *ManifestCheck `json:"manifest"`
	Repositories []RepoCheck    `json:"repositories"`
	Errors       []string       `json:"errors,omitempty"`
	Summary      VerifySummary  `json:"summary"`
}

// ManifestCheck represents manifest verification.
type ManifestCheck struct {
	Exists    bool   `json:"exists"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	RepoCount int    `json:"repo_count,omitempty"`
}

// RepoCheck represents a repository verification.
type RepoCheck struct {
	Slug       string      `json:"slug"`
	Project    string      `json:"project,omitempty"`
	GitCheck   *GitCheck   `json:"git,omitempty"`
	JSONChecks []JSONCheck `json:"json_checks,omitempty"`
	Valid      bool        `json:"valid"`
	Errors     []string    `json:"errors,omitempty"`
}

// GitCheck represents git fsck result.
type GitCheck struct {
	Exists bool   `json:"exists"`
	Valid  bool   `json:"valid"`
	Error  string `json:"error,omitempty"`
}

// JSONCheck represents a JSON file validation.
type JSONCheck struct {
	File  string `json:"file"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// VerifySummary contains summary statistics.
type VerifySummary struct {
	TotalRepos   int `json:"total_repos"`
	ValidRepos   int `json:"valid_repos"`
	InvalidRepos int `json:"invalid_repos"`
	TotalGit     int `json:"total_git"`
	ValidGit     int `json:"valid_git"`
	TotalJSON    int `json:"total_json"`
	ValidJSON    int `json:"valid_json"`
}

// Manifest represents the backup manifest structure.
type Manifest struct {
	Workspace    string `json:"workspace"`
	Timestamp    string `json:"timestamp"`
	Repositories []struct {
		Slug    string `json:"slug"`
		Project string `json:"project,omitempty"`
	} `json:"repositories"`
}

func runVerify(_ *cobra.Command, args []string) error {
	backupPath := args[0]

	result := &VerifyResult{
		Path:         backupPath,
		Valid:        true,
		Repositories: make([]RepoCheck, 0),
		Errors:       make([]string, 0),
	}

	// Check if backup path exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("backup path does not exist: %s", backupPath))
		return outputVerifyResult(result)
	}

	// Check manifest
	result.Manifest = verifyManifest(backupPath)
	if !result.Manifest.Valid {
		result.Valid = false
	}

	// If manifest is valid, verify repositories from it
	if result.Manifest.Valid && result.Manifest.RepoCount > 0 {
		verifyRepositoriesFromManifest(backupPath, result)
	} else {
		// Fall back to scanning directory structure
		verifyRepositoriesFromDirectory(backupPath, result)
	}

	// Calculate summary
	for _, repo := range result.Repositories {
		result.Summary.TotalRepos++
		if repo.Valid {
			result.Summary.ValidRepos++
		} else {
			result.Summary.InvalidRepos++
			result.Valid = false
		}

		if repo.GitCheck != nil {
			result.Summary.TotalGit++
			if repo.GitCheck.Valid {
				result.Summary.ValidGit++
			}
		}

		for _, jc := range repo.JSONChecks {
			result.Summary.TotalJSON++
			if jc.Valid {
				result.Summary.ValidJSON++
			}
		}
	}

	return outputVerifyResult(result)
}

func verifyManifest(backupPath string) *ManifestCheck {
	check := &ManifestCheck{}

	manifestPath := filepath.Join(backupPath, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			check.Exists = false
			check.Valid = false
			check.Error = "manifest.json not found"
		} else {
			check.Exists = true
			check.Valid = false
			check.Error = fmt.Sprintf("failed to read manifest: %v", err)
		}
		return check
	}

	check.Exists = true

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		check.Valid = false
		check.Error = fmt.Sprintf("invalid JSON: %v", err)
		return check
	}

	check.Valid = true
	check.Workspace = manifest.Workspace
	check.Timestamp = manifest.Timestamp
	check.RepoCount = len(manifest.Repositories)

	return check
}

func verifyRepositoriesFromManifest(backupPath string, result *VerifyResult) {
	manifestPath := filepath.Join(backupPath, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return
	}

	for _, repo := range manifest.Repositories {
		var repoPath string
		if repo.Project != "" {
			repoPath = filepath.Join(backupPath, "projects", repo.Project, "repositories", repo.Slug)
		} else {
			repoPath = filepath.Join(backupPath, "personal", "repositories", repo.Slug)
		}

		repoCheck := verifyRepository(repoPath, repo.Slug, repo.Project)
		result.Repositories = append(result.Repositories, repoCheck)
	}
}

func verifyRepositoriesFromDirectory(backupPath string, result *VerifyResult) {
	// Scan projects directory
	projectsPath := filepath.Join(backupPath, "projects")
	if entries, err := os.ReadDir(projectsPath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				projectKey := entry.Name()
				reposPath := filepath.Join(projectsPath, projectKey, "repositories")
				if repoEntries, err := os.ReadDir(reposPath); err == nil {
					for _, repoEntry := range repoEntries {
						if repoEntry.IsDir() {
							repoPath := filepath.Join(reposPath, repoEntry.Name())
							repoCheck := verifyRepository(repoPath, repoEntry.Name(), projectKey)
							result.Repositories = append(result.Repositories, repoCheck)
						}
					}
				}
			}
		}
	}

	// Scan personal repos
	personalPath := filepath.Join(backupPath, "personal", "repositories")
	if entries, err := os.ReadDir(personalPath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				repoPath := filepath.Join(personalPath, entry.Name())
				repoCheck := verifyRepository(repoPath, entry.Name(), "")
				result.Repositories = append(result.Repositories, repoCheck)
			}
		}
	}
}

func verifyRepository(repoPath, slug, project string) RepoCheck {
	check := RepoCheck{
		Slug:       slug,
		Project:    project,
		Valid:      true,
		JSONChecks: make([]JSONCheck, 0),
		Errors:     make([]string, 0),
	}

	// Check if repo directory exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		check.Valid = false
		check.Errors = append(check.Errors, "repository directory not found")
		return check
	}

	// Check git repository
	gitPath := filepath.Join(repoPath, "repo.git")
	check.GitCheck = verifyGitRepo(gitPath)
	if !check.GitCheck.Valid {
		check.Valid = false
		check.Errors = append(check.Errors, fmt.Sprintf("git: %s", check.GitCheck.Error))
	}

	// Check JSON files
	jsonFiles := []string{
		"repository.json",
	}

	// Check for PR and issue directories
	prDir := filepath.Join(repoPath, "pull-requests")
	if _, err := os.Stat(prDir); err == nil {
		// Check all PR JSON files
		entries, _ := os.ReadDir(prDir)
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				jsonFiles = append(jsonFiles, filepath.Join("pull-requests", entry.Name()))
			}
			if entry.IsDir() {
				// Check comments.json and activity.json
				prSubDir := filepath.Join("pull-requests", entry.Name())
				for _, subFile := range []string{"comments.json", "activity.json"} {
					subPath := filepath.Join(prSubDir, subFile)
					if _, err := os.Stat(filepath.Join(repoPath, subPath)); err == nil {
						jsonFiles = append(jsonFiles, subPath)
					}
				}
			}
		}
	}

	issueDir := filepath.Join(repoPath, "issues")
	if _, err := os.Stat(issueDir); err == nil {
		entries, _ := os.ReadDir(issueDir)
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				jsonFiles = append(jsonFiles, filepath.Join("issues", entry.Name()))
			}
			if entry.IsDir() {
				commentsPath := filepath.Join("issues", entry.Name(), "comments.json")
				if _, err := os.Stat(filepath.Join(repoPath, commentsPath)); err == nil {
					jsonFiles = append(jsonFiles, commentsPath)
				}
			}
		}
	}

	for _, jsonFile := range jsonFiles {
		jc := verifyJSONFile(filepath.Join(repoPath, jsonFile), jsonFile)
		check.JSONChecks = append(check.JSONChecks, jc)
		if !jc.Valid {
			check.Valid = false
			check.Errors = append(check.Errors, fmt.Sprintf("json %s: %s", jsonFile, jc.Error))
		}
	}

	return check
}

func verifyGitRepo(gitPath string) *GitCheck {
	check := &GitCheck{}

	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		check.Exists = false
		check.Valid = false
		check.Error = "repo.git directory not found"
		return check
	}

	check.Exists = true

	// Run git fsck
	cmd := exec.Command("git", "fsck", "--no-dangling")
	cmd.Dir = gitPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		check.Valid = false
		check.Error = fmt.Sprintf("git fsck failed: %s", strings.TrimSpace(string(output)))
		return check
	}

	check.Valid = true
	return check
}

func verifyJSONFile(filePath, relPath string) JSONCheck {
	check := JSONCheck{
		File: relPath,
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			check.Valid = false
			check.Error = "file not found"
		} else {
			check.Valid = false
			check.Error = fmt.Sprintf("read error: %v", err)
		}
		return check
	}

	var js json.RawMessage
	if err := json.Unmarshal(data, &js); err != nil {
		check.Valid = false
		check.Error = fmt.Sprintf("invalid JSON: %v", err)
		return check
	}

	check.Valid = true
	return check
}

func outputVerifyResult(result *VerifyResult) error {
	if verifyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	} else {
		outputVerifyText(result)
	}

	if !result.Valid {
		os.Exit(1)
	}
	return nil
}

func outputVerifyText(result *VerifyResult) {
	fmt.Printf("Verifying backup: %s\n\n", result.Path)

	// Manifest
	fmt.Println("Manifest:")
	if result.Manifest != nil {
		if result.Manifest.Valid {
			fmt.Printf("  ✓ manifest.json (workspace: %s, repos: %d)\n", result.Manifest.Workspace, result.Manifest.RepoCount)
		} else {
			fmt.Printf("  ✗ manifest.json: %s\n", result.Manifest.Error)
		}
	}

	// Repositories
	fmt.Printf("\nRepositories (%d):\n", len(result.Repositories))
	for _, repo := range result.Repositories {
		status := "✓"
		if !repo.Valid {
			status = "✗"
		}

		projectInfo := ""
		if repo.Project != "" {
			projectInfo = fmt.Sprintf(" [%s]", repo.Project)
		}

		fmt.Printf("  %s %s%s\n", status, repo.Slug, projectInfo)

		if verifyVerbose || !repo.Valid {
			if repo.GitCheck != nil {
				gitStatus := "✓"
				if !repo.GitCheck.Valid {
					gitStatus = "✗"
				}
				if repo.GitCheck.Exists {
					fmt.Printf("      git: %s\n", gitStatus)
					if !repo.GitCheck.Valid {
						fmt.Printf("           %s\n", repo.GitCheck.Error)
					}
				} else {
					fmt.Printf("      git: ✗ not found\n")
				}
			}

			if verifyVerbose {
				for _, jc := range repo.JSONChecks {
					jsonStatus := "✓"
					if !jc.Valid {
						jsonStatus = "✗"
					}
					fmt.Printf("      %s: %s\n", jc.File, jsonStatus)
					if !jc.Valid {
						fmt.Printf("           %s\n", jc.Error)
					}
				}
			}
		}
	}

	// Summary
	fmt.Println("\nSummary:")
	fmt.Printf("  Repositories: %d valid, %d invalid\n", result.Summary.ValidRepos, result.Summary.InvalidRepos)
	fmt.Printf("  Git repos:    %d/%d valid\n", result.Summary.ValidGit, result.Summary.TotalGit)
	fmt.Printf("  JSON files:   %d/%d valid\n", result.Summary.ValidJSON, result.Summary.TotalJSON)

	fmt.Println()
	if result.Valid {
		fmt.Println("Result: PASS")
	} else {
		fmt.Println("Result: FAIL")
	}
}
