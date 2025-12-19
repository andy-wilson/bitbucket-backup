package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Repository represents a Bitbucket repository.
type Repository struct {
	Type        string   `json:"type"`
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	IsPrivate   bool     `json:"is_private"`
	ForkPolicy  string   `json:"fork_policy"`
	Language    string   `json:"language"`
	HasIssues   bool     `json:"has_issues"`
	HasWiki     bool     `json:"has_wiki"`
	SCM         string   `json:"scm"`
	Size        int64    `json:"size"`
	Links       Links    `json:"links"`
	Project     *Project `json:"project,omitempty"`
	MainBranch  *Branch  `json:"mainbranch,omitempty"`
	Owner       *User    `json:"owner,omitempty"`
	CreatedOn   string   `json:"created_on"`
	UpdatedOn   string   `json:"updated_on"`
}

// Branch represents a git branch.
type Branch struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// GetRepositories fetches all repositories in a workspace.
func (c *Client) GetRepositories(ctx context.Context, workspace string) ([]Repository, error) {
	path := fmt.Sprintf("/repositories/%s", workspace)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching repositories for workspace %s: %w", workspace, err)
	}

	repos := make([]Repository, 0, len(values))
	for _, v := range values {
		var r Repository
		if err := json.Unmarshal(v, &r); err != nil {
			return nil, fmt.Errorf("parsing repository: %w", err)
		}
		repos = append(repos, r)
	}

	return repos, nil
}

// GetRepository fetches a single repository.
func (c *Client) GetRepository(ctx context.Context, workspace, repoSlug string) (*Repository, error) {
	path := fmt.Sprintf("/repositories/%s/%s", workspace, repoSlug)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching repository %s/%s: %w", workspace, repoSlug, err)
	}

	var r Repository
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parsing repository response: %w", err)
	}

	return &r, nil
}

// GetProjectRepositories fetches all repositories in a specific project.
func (c *Client) GetProjectRepositories(ctx context.Context, workspace, projectKey string) ([]Repository, error) {
	// Use query parameter to filter by project
	path := fmt.Sprintf("/repositories/%s?q=project.key=\"%s\"", workspace, projectKey)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching repositories for project %s/%s: %w", workspace, projectKey, err)
	}

	repos := make([]Repository, 0, len(values))
	for _, v := range values {
		var r Repository
		if err := json.Unmarshal(v, &r); err != nil {
			return nil, fmt.Errorf("parsing repository: %w", err)
		}
		repos = append(repos, r)
	}

	return repos, nil
}

// GetPersonalRepositories fetches repositories that don't belong to any project.
func (c *Client) GetPersonalRepositories(ctx context.Context, workspace string) ([]Repository, error) {
	// Fetch all repositories and filter those without a project
	allRepos, err := c.GetRepositories(ctx, workspace)
	if err != nil {
		return nil, err
	}

	var personalRepos []Repository
	for _, r := range allRepos {
		if r.Project == nil {
			personalRepos = append(personalRepos, r)
		}
	}

	return personalRepos, nil
}

// CloneURL returns the HTTPS clone URL for a repository.
func (r *Repository) CloneURL() string {
	for _, link := range r.Links.Clone {
		if link.Name == "https" {
			return link.Href
		}
	}
	return ""
}
