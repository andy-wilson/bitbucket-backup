package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Project represents a Bitbucket project.
type Project struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	Links       Links  `json:"links"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	Owner       *User  `json:"owner,omitempty"`
}

// User represents a Bitbucket user.
type User struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	AccountID   string `json:"account_id"`
	Links       Links  `json:"links"`
}

// GetProjects fetches all projects in a workspace.
func (c *Client) GetProjects(ctx context.Context, workspace string) ([]Project, error) {
	path := fmt.Sprintf("/workspaces/%s/projects", workspace)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching projects for workspace %s: %w", workspace, err)
	}

	projects := make([]Project, 0, len(values))
	for _, v := range values {
		var p Project
		if err := json.Unmarshal(v, &p); err != nil {
			return nil, fmt.Errorf("parsing project: %w", err)
		}
		projects = append(projects, p)
	}

	return projects, nil
}

// GetProject fetches a single project by key.
func (c *Client) GetProject(ctx context.Context, workspace, projectKey string) (*Project, error) {
	path := fmt.Sprintf("/workspaces/%s/projects/%s", workspace, projectKey)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching project %s/%s: %w", workspace, projectKey, err)
	}

	var p Project
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parsing project response: %w", err)
	}

	return &p, nil
}
