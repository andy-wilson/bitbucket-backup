package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Issue represents a Bitbucket issue.
type Issue struct {
	Type       string       `json:"type"`
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	Reporter   *User        `json:"reporter"`
	Assignee   *User        `json:"assignee,omitempty"`
	State      string       `json:"state"`
	Kind       string       `json:"kind"`
	Priority   string       `json:"priority"`
	Milestone  *Milestone   `json:"milestone,omitempty"`
	Version    *Version     `json:"version,omitempty"`
	Component  *Component   `json:"component,omitempty"`
	Votes      int          `json:"votes"`
	Watches    int          `json:"watches"`
	Content    *Content     `json:"content,omitempty"`
	CreatedOn  string       `json:"created_on"`
	UpdatedOn  string       `json:"updated_on"`
	EditedOn   string       `json:"edited_on,omitempty"`
	Links      Links        `json:"links"`
	Repository *Repository  `json:"repository,omitempty"`
}

// Milestone represents a project milestone.
type Milestone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Version represents a project version.
type Version struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Component represents a project component.
type Component struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// IssueComment represents a comment on an issue.
type IssueComment struct {
	Type      string   `json:"type"`
	ID        int      `json:"id"`
	CreatedOn string   `json:"created_on"`
	UpdatedOn string   `json:"updated_on"`
	Content   *Content `json:"content"`
	User      *User    `json:"user"`
	Issue     *Issue   `json:"issue,omitempty"`
	Links     Links    `json:"links"`
}

// IssueChange represents a change to an issue.
type IssueChange struct {
	Type      string             `json:"type"`
	ID        int                `json:"id"`
	CreatedOn string             `json:"created_on"`
	User      *User              `json:"user"`
	Issue     *Issue             `json:"issue,omitempty"`
	Changes   *IssueChangeDetail `json:"changes,omitempty"`
	Message   *Content           `json:"message,omitempty"`
	Links     Links              `json:"links"`
}

// IssueChangeDetail contains the specific changes made.
type IssueChangeDetail struct {
	State     *ChangeValue `json:"state,omitempty"`
	Title     *ChangeValue `json:"title,omitempty"`
	Kind      *ChangeValue `json:"kind,omitempty"`
	Priority  *ChangeValue `json:"priority,omitempty"`
	Assignee  *ChangeValue `json:"assignee,omitempty"`
	Component *ChangeValue `json:"component,omitempty"`
	Milestone *ChangeValue `json:"milestone,omitempty"`
	Version   *ChangeValue `json:"version,omitempty"`
	Content   *ChangeValue `json:"content,omitempty"`
}

// ChangeValue represents old and new values for a change.
type ChangeValue struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// GetIssues fetches all issues for a repository.
// Returns empty slice if issue tracker is disabled.
func (c *Client) GetIssues(ctx context.Context, workspace, repoSlug string) ([]Issue, error) {
	path := fmt.Sprintf("/repositories/%s/%s/issues", workspace, repoSlug)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		// Check if it's a 404 - issue tracker might be disabled
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			return []Issue{}, nil
		}
		return nil, fmt.Errorf("fetching issues for %s/%s: %w", workspace, repoSlug, err)
	}

	issues := make([]Issue, 0, len(values))
	for _, v := range values {
		var issue Issue
		if err := json.Unmarshal(v, &issue); err != nil {
			return nil, fmt.Errorf("parsing issue: %w", err)
		}
		issues = append(issues, issue)
	}

	return issues, nil
}

// GetIssue fetches a single issue by ID.
func (c *Client) GetIssue(ctx context.Context, workspace, repoSlug string, issueID int) (*Issue, error) {
	path := fmt.Sprintf("/repositories/%s/%s/issues/%d", workspace, repoSlug, issueID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching issue %d: %w", issueID, err)
	}

	var issue Issue
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, fmt.Errorf("parsing issue: %w", err)
	}

	return &issue, nil
}

// GetIssueComments fetches all comments on an issue.
func (c *Client) GetIssueComments(ctx context.Context, workspace, repoSlug string, issueID int) ([]IssueComment, error) {
	path := fmt.Sprintf("/repositories/%s/%s/issues/%d/comments", workspace, repoSlug, issueID)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching issue comments: %w", err)
	}

	comments := make([]IssueComment, 0, len(values))
	for _, v := range values {
		var comment IssueComment
		if err := json.Unmarshal(v, &comment); err != nil {
			return nil, fmt.Errorf("parsing issue comment: %w", err)
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

// GetIssueChanges fetches the change history for an issue.
func (c *Client) GetIssueChanges(ctx context.Context, workspace, repoSlug string, issueID int) ([]IssueChange, error) {
	path := fmt.Sprintf("/repositories/%s/%s/issues/%d/changes", workspace, repoSlug, issueID)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching issue changes: %w", err)
	}

	changes := make([]IssueChange, 0, len(values))
	for _, v := range values {
		var change IssueChange
		if err := json.Unmarshal(v, &change); err != nil {
			return nil, fmt.Errorf("parsing issue change: %w", err)
		}
		changes = append(changes, change)
	}

	return changes, nil
}

// GetIssuesUpdatedSince fetches issues updated after the given timestamp.
// Useful for incremental backups.
func (c *Client) GetIssuesUpdatedSince(ctx context.Context, workspace, repoSlug, since string) ([]Issue, error) {
	path := fmt.Sprintf("/repositories/%s/%s/issues?q=updated_on>%%22%s%%22", workspace, repoSlug, since)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		// Check if it's a 404 - issue tracker might be disabled
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			return []Issue{}, nil
		}
		return nil, fmt.Errorf("fetching updated issues: %w", err)
	}

	issues := make([]Issue, 0, len(values))
	for _, v := range values {
		var issue Issue
		if err := json.Unmarshal(v, &issue); err != nil {
			return nil, fmt.Errorf("parsing issue: %w", err)
		}
		issues = append(issues, issue)
	}

	return issues, nil
}
