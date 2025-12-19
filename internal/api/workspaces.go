package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Workspace represents a Bitbucket workspace.
type Workspace struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	IsPrivate bool   `json:"is_private"`
	Links     Links  `json:"links"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
}

// Links contains hypermedia links.
type Links struct {
	Self         Link   `json:"self"`
	HTML         Link   `json:"html"`
	Avatar       Link   `json:"avatar"`
	Repositories Link   `json:"repositories"`
	Projects     Link   `json:"projects"`
	Clone        []Link `json:"clone"`
}

// Link represents a single hypermedia link.
type Link struct {
	Href string `json:"href"`
	Name string `json:"name,omitempty"`
}

// GetWorkspace fetches metadata for a workspace.
func (c *Client) GetWorkspace(ctx context.Context, workspace string) (*Workspace, error) {
	path := fmt.Sprintf("/workspaces/%s", workspace)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching workspace %s: %w", workspace, err)
	}

	var ws Workspace
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("parsing workspace response: %w", err)
	}

	return &ws, nil
}
