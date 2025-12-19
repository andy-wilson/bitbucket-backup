package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/andy-wilson/bb-backup/internal/api"
	"github.com/andy-wilson/bb-backup/internal/backup"
	"github.com/andy-wilson/bb-backup/internal/config"
	"github.com/spf13/cobra"
)

var (
	listJSON            bool
	listExcludeRepos    []string
	listIncludeRepos    []string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List repositories that would be backed up",
	Long: `List all projects and repositories in the workspace that would be backed up.

This is useful for previewing what a backup will include without actually
performing the backup. Filtering patterns from config and CLI are applied.

Output formats:
  (default)    Human-readable text output
  --json       Machine-readable JSON output

Repository filtering:
  --include "pattern"  Only include repos matching glob pattern
  --exclude "pattern"  Exclude repos matching glob pattern
  Patterns support * and ? wildcards (e.g., "core-*", "test-?-*")

Examples:
  bb-backup list -c config.yaml
  bb-backup list -w my-workspace --username user --app-password $TOKEN
  bb-backup list --json
  bb-backup list --exclude "test-*" --exclude "archive-*"
  bb-backup list --include "core-*" -v`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	// Re-use auth flags from backup command
	listCmd.Flags().StringVar(&username, "username", "", "Bitbucket username")
	listCmd.Flags().StringVar(&appPassword, "app-password", "", "Bitbucket app password")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	listCmd.Flags().StringArrayVar(&listExcludeRepos, "exclude", nil, "exclude repos matching glob pattern")
	listCmd.Flags().StringArrayVar(&listIncludeRepos, "include", nil, "only include repos matching glob pattern")
}

// ListOutput represents the JSON output for the list command.
type ListOutput struct {
	Workspace    string              `json:"workspace"`
	Projects     []ProjectOutput     `json:"projects"`
	Personal     []RepositoryOutput  `json:"personal"`
	TotalRepos   int                 `json:"total_repos"`
	FilteredOut  int                 `json:"filtered_out"`
}

// ProjectOutput represents a project in JSON output.
type ProjectOutput struct {
	Key          string             `json:"key"`
	Name         string             `json:"name"`
	Repositories []RepositoryOutput `json:"repositories"`
}

// RepositoryOutput represents a repository in JSON output.
type RepositoryOutput struct {
	Slug        string `json:"slug"`
	FullName    string `json:"full_name"`
	Description string `json:"description,omitempty"`
	IsPrivate   bool   `json:"is_private"`
	Size        int64  `json:"size,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadListConfig()
	if err != nil {
		return err
	}

	// Apply filter overrides from CLI
	if len(listExcludeRepos) > 0 {
		cfg.Backup.ExcludeRepos = mergePatterns(cfg.Backup.ExcludeRepos, listExcludeRepos)
	}
	if len(listIncludeRepos) > 0 {
		cfg.Backup.IncludeRepos = mergePatterns(cfg.Backup.IncludeRepos, listIncludeRepos)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	client := api.NewClient(cfg)

	// Fetch projects
	projects, err := client.GetProjects(ctx, cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching projects: %w", err)
	}

	// Fetch all repositories
	allRepos, err := client.GetRepositories(ctx, cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching repositories: %w", err)
	}

	// Apply filters
	filter := backup.NewRepoFilter(cfg.Backup.IncludeRepos, cfg.Backup.ExcludeRepos)
	repos := filter.Filter(allRepos)
	filteredOut := len(allRepos) - len(repos)

	// Group repos by project
	reposByProject := make(map[string][]api.Repository)
	var personalRepos []api.Repository

	for _, repo := range repos {
		if repo.Project != nil {
			reposByProject[repo.Project.Key] = append(reposByProject[repo.Project.Key], repo)
		} else {
			personalRepos = append(personalRepos, repo)
		}
	}

	if listJSON {
		return outputListJSON(cfg.Workspace, projects, reposByProject, personalRepos, len(repos), filteredOut)
	}

	return outputListText(cfg.Workspace, projects, reposByProject, personalRepos, len(repos), filteredOut)
}

func outputListJSON(workspace string, projects []api.Project, reposByProject map[string][]api.Repository, personalRepos []api.Repository, totalRepos, filteredOut int) error {
	output := ListOutput{
		Workspace:   workspace,
		Projects:    make([]ProjectOutput, 0, len(projects)),
		Personal:    make([]RepositoryOutput, 0, len(personalRepos)),
		TotalRepos:  totalRepos,
		FilteredOut: filteredOut,
	}

	for _, project := range projects {
		projectRepos := reposByProject[project.Key]
		po := ProjectOutput{
			Key:          project.Key,
			Name:         project.Name,
			Repositories: make([]RepositoryOutput, 0, len(projectRepos)),
		}
		for _, repo := range projectRepos {
			po.Repositories = append(po.Repositories, RepositoryOutput{
				Slug:        repo.Slug,
				FullName:    repo.FullName,
				Description: repo.Description,
				IsPrivate:   repo.IsPrivate,
				Size:        repo.Size,
			})
		}
		output.Projects = append(output.Projects, po)
	}

	for _, repo := range personalRepos {
		output.Personal = append(output.Personal, RepositoryOutput{
			Slug:        repo.Slug,
			FullName:    repo.FullName,
			Description: repo.Description,
			IsPrivate:   repo.IsPrivate,
			Size:        repo.Size,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputListText(workspace string, projects []api.Project, reposByProject map[string][]api.Repository, personalRepos []api.Repository, totalRepos, filteredOut int) error {
	fmt.Printf("Workspace: %s\n\n", workspace)

	// Print projects and their repos
	fmt.Printf("Projects (%d):\n", len(projects))
	for _, project := range projects {
		projectRepos := reposByProject[project.Key]
		fmt.Printf("  %s (%s) - %d repositories\n", project.Name, project.Key, len(projectRepos))

		if verbose {
			for _, repo := range projectRepos {
				fmt.Printf("    - %s\n", repo.Slug)
			}
		}
	}

	// Print personal repos
	if len(personalRepos) > 0 {
		fmt.Printf("\nPersonal repositories (%d):\n", len(personalRepos))
		for _, repo := range personalRepos {
			fmt.Printf("  - %s\n", repo.Slug)
		}
	}

	// Summary
	fmt.Printf("\nTotal: %d projects, %d repositories\n", len(projects), totalRepos)
	if filteredOut > 0 {
		fmt.Printf("Filtered out: %d repositories (by include/exclude patterns)\n", filteredOut)
	}

	return nil
}

func loadListConfig() (*config.Config, error) {
	cfgPath := getConfigPath()

	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("loading config from %s: %w", cfgPath, err)
		}
		// Apply workspace override if specified
		if workspace != "" {
			cfg.Workspace = workspace
		}
		// Apply auth overrides
		if username != "" {
			cfg.Auth.Username = username
		}
		if appPassword != "" {
			cfg.Auth.AppPassword = appPassword
		}
		return cfg, nil
	}

	// No config file - build from flags
	if workspace == "" {
		workspace = os.Getenv("BITBUCKET_WORKSPACE")
	}
	if workspace == "" {
		return nil, fmt.Errorf("no config file found and --workspace not specified")
	}

	cfg := config.Default()
	cfg.Workspace = workspace

	if username == "" {
		username = os.Getenv("BITBUCKET_USERNAME")
	}
	if appPassword == "" {
		appPassword = os.Getenv("BITBUCKET_APP_PASSWORD")
	}

	cfg.Auth.Username = username
	cfg.Auth.AppPassword = appPassword

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
