package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// PullRequest represents a Bitbucket pull request.
type PullRequest struct {
	Type              string        `json:"type"`
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	State             string        `json:"state"`
	MergeCommit       *Commit       `json:"merge_commit,omitempty"`
	CloseSourceBranch bool          `json:"close_source_branch"`
	ClosedBy          *User         `json:"closed_by,omitempty"`
	Author            *User         `json:"author"`
	Reason            string        `json:"reason"`
	CreatedOn         string        `json:"created_on"`
	UpdatedOn         string        `json:"updated_on"`
	Destination       *PREndpoint   `json:"destination"`
	Source            *PREndpoint   `json:"source"`
	Links             Links         `json:"links"`
	Summary           *PRSummary    `json:"summary,omitempty"`
	Reviewers         []User        `json:"reviewers,omitempty"`
	Participants      []Participant `json:"participants,omitempty"`
	TaskCount         int           `json:"task_count"`
	CommentCount      int           `json:"comment_count"`
}

// Commit represents a git commit.
type Commit struct {
	Type    string `json:"type"`
	Hash    string `json:"hash"`
	Date    string `json:"date,omitempty"`
	Author  *User  `json:"author,omitempty"`
	Message string `json:"message,omitempty"`
	Links   Links  `json:"links"`
}

// PREndpoint represents the source or destination of a PR.
type PREndpoint struct {
	Repository *Repository `json:"repository"`
	Branch     *Branch     `json:"branch"`
	Commit     *Commit     `json:"commit"`
}

// PRSummary contains the rendered PR description.
type PRSummary struct {
	Type   string `json:"type"`
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
}

// Participant represents a PR participant.
type Participant struct {
	Type           string `json:"type"`
	User           *User  `json:"user"`
	Role           string `json:"role"`
	Approved       bool   `json:"approved"`
	State          string `json:"state"`
	ParticipatedOn string `json:"participated_on"`
}

// PRComment represents a comment on a pull request.
type PRComment struct {
	Type      string     `json:"type"`
	ID        int        `json:"id"`
	CreatedOn string     `json:"created_on"`
	UpdatedOn string     `json:"updated_on"`
	Content   *Content   `json:"content"`
	User      *User      `json:"user"`
	Deleted   bool       `json:"deleted"`
	Parent    *PRComment `json:"parent,omitempty"`
	Inline    *Inline    `json:"inline,omitempty"`
	Links     Links      `json:"links"`
}

// Content represents rendered content.
type Content struct {
	Type   string `json:"type"`
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
}

// Inline represents inline comment position.
type Inline struct {
	From *int   `json:"from,omitempty"`
	To   *int   `json:"to,omitempty"`
	Path string `json:"path"`
}

// PRActivity represents an activity entry on a PR.
type PRActivity struct {
	Type     string      `json:"type,omitempty"`
	Approval *PRApproval `json:"approval,omitempty"`
	Update   *PRUpdate   `json:"update,omitempty"`
	Comment  *PRComment  `json:"comment,omitempty"`
	Changes  *PRChanges  `json:"changes_requested,omitempty"`
}

// PRApproval represents an approval on a PR.
type PRApproval struct {
	Date string `json:"date"`
	User *User  `json:"user"`
}

// PRUpdate represents a PR update event.
type PRUpdate struct {
	Date        string      `json:"date"`
	Author      *User       `json:"author"`
	Source      *PREndpoint `json:"source,omitempty"`
	Destination *PREndpoint `json:"destination,omitempty"`
	Title       string      `json:"title,omitempty"`
	Description string      `json:"description,omitempty"`
	Reason      string      `json:"reason,omitempty"`
	State       string      `json:"state,omitempty"`
	Reviewers   []User      `json:"reviewers,omitempty"`
}

// PRChanges represents a changes requested event.
type PRChanges struct {
	Date string `json:"date"`
	User *User  `json:"user"`
}

// GetPullRequests fetches all pull requests for a repository.
// State can be: OPEN, MERGED, DECLINED, SUPERSEDED, or empty for all.
func (c *Client) GetPullRequests(ctx context.Context, workspace, repoSlug, state string) ([]PullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests", workspace, repoSlug)
	if state != "" {
		path = fmt.Sprintf("%s?state=%s", path, state)
	}

	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching pull requests for %s/%s: %w", workspace, repoSlug, err)
	}

	prs := make([]PullRequest, 0, len(values))
	for _, v := range values {
		var pr PullRequest
		if err := json.Unmarshal(v, &pr); err != nil {
			return nil, fmt.Errorf("parsing pull request: %w", err)
		}
		prs = append(prs, pr)
	}

	return prs, nil
}

// GetAllPullRequests fetches all pull requests in all states concurrently.
func (c *Client) GetAllPullRequests(ctx context.Context, workspace, repoSlug string) ([]PullRequest, error) {
	states := []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}

	type result struct {
		prs []PullRequest
		err error
	}

	results := make([]result, len(states))
	var wg sync.WaitGroup

	// Fetch all states concurrently
	for i, state := range states {
		wg.Add(1)
		go func(idx int, st string) {
			defer wg.Done()
			prs, err := c.GetPullRequests(ctx, workspace, repoSlug, st)
			results[idx] = result{prs: prs, err: err}
		}(i, state)
	}

	wg.Wait()

	// Collect results and check for errors
	var allPRs []PullRequest
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		allPRs = append(allPRs, r.prs...)
	}

	return allPRs, nil
}

// GetPullRequest fetches a single pull request by ID.
func (c *Client) GetPullRequest(ctx context.Context, workspace, repoSlug string, prID int) (*PullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repoSlug, prID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching pull request %d: %w", prID, err)
	}

	var pr PullRequest
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("parsing pull request: %w", err)
	}

	return &pr, nil
}

// GetPullRequestComments fetches all comments on a pull request.
func (c *Client) GetPullRequestComments(ctx context.Context, workspace, repoSlug string, prID int) ([]PRComment, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching PR comments: %w", err)
	}

	comments := make([]PRComment, 0, len(values))
	for _, v := range values {
		var comment PRComment
		if err := json.Unmarshal(v, &comment); err != nil {
			return nil, fmt.Errorf("parsing PR comment: %w", err)
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

// GetPullRequestActivity fetches all activity on a pull request.
func (c *Client) GetPullRequestActivity(ctx context.Context, workspace, repoSlug string, prID int) ([]PRActivity, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/activity", workspace, repoSlug, prID)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching PR activity: %w", err)
	}

	activities := make([]PRActivity, 0, len(values))
	for _, v := range values {
		var activity PRActivity
		if err := json.Unmarshal(v, &activity); err != nil {
			return nil, fmt.Errorf("parsing PR activity: %w", err)
		}
		activities = append(activities, activity)
	}

	return activities, nil
}

// GetPullRequestsUpdatedSince fetches PRs updated after the given timestamp.
// Useful for incremental backups.
func (c *Client) GetPullRequestsUpdatedSince(ctx context.Context, workspace, repoSlug, since string) ([]PullRequest, error) {
	// Use query parameter to filter by updated_on
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests?q=updated_on>%%22%s%%22", workspace, repoSlug, since)
	values, err := c.GetPaginated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetching updated pull requests: %w", err)
	}

	prs := make([]PullRequest, 0, len(values))
	for _, v := range values {
		var pr PullRequest
		if err := json.Unmarshal(v, &pr); err != nil {
			return nil, fmt.Errorf("parsing pull request: %w", err)
		}
		prs = append(prs, pr)
	}

	return prs, nil
}
