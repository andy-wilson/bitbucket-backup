package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/andy-wilson/bb-backup/internal/api"
	"github.com/andy-wilson/bb-backup/internal/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List repositories that would be backed up",
	Long: `List all projects and repositories in the workspace that would be backed up.

This is useful for previewing what a backup will include without actually
performing the backup.

Examples:
  bb-backup list -c config.yaml
  bb-backup list -w my-workspace --username user --app-password $TOKEN`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	// Re-use auth flags from backup command
	listCmd.Flags().StringVar(&username, "username", "", "Bitbucket username")
	listCmd.Flags().StringVar(&appPassword, "app-password", "", "Bitbucket app password")
}

func runList(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadListConfig()
	if err != nil {
		return err
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

	// Fetch workspace
	fmt.Printf("Workspace: %s\n\n", cfg.Workspace)

	// Fetch projects
	projects, err := client.GetProjects(ctx, cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching projects: %w", err)
	}

	// Fetch all repositories
	repos, err := client.GetRepositories(ctx, cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching repositories: %w", err)
	}

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
	fmt.Printf("\nTotal: %d projects, %d repositories\n", len(projects), len(repos))

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
