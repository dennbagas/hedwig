package githubapp

import (
	"context"
	"net/http"
)

type Client interface {
	ValidateWebhook(r *http.Request) (payload []byte, err error)
	ParseWebhook(eventType string, payload []byte) (any, error)
	RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error
	// CreatePR returns the HTML URL of the newly created pull request.
	CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (string, error)
	// ListPullRequestReviews returns every review on a pull request, in the
	// order GitHub returns them (chronological). Used by the approval gate.
	ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]Review, error)
	// IsTeamMember reports whether username is an active member of org/teamSlug.
	// A non-member (GitHub 404) returns (false, nil), not an error.
	IsTeamMember(ctx context.Context, org, teamSlug, username string) (bool, error)
	// GetBranchApprovalRules returns the approval requirements of every
	// ruleset (repo- or org-level) active on branch. A branch with no
	// pull_request rule returns a zero value, not an error.
	GetBranchApprovalRules(ctx context.Context, owner, repo, branch string) (BranchApprovalRules, error)
	// GetTeamSlug resolves a team ID (as rulesets report it) to its slug.
	GetTeamSlug(ctx context.Context, orgID, teamID int64) (string, error)
}

// BranchApprovalRules aggregates the pull_request rules active on a branch.
type BranchApprovalRules struct {
	// RequiredApprovals is the strictest required_approving_review_count
	// across all active pull_request rules; 0 means none is required.
	RequiredApprovals int
	// Teams lists every required_reviewers entry whose reviewer is a team.
	Teams []TeamApprovalRule
}

// TeamApprovalRule is one ruleset requirement for approvals from a team.
type TeamApprovalRule struct {
	TeamID       int64
	MinApprovals int
}

// Review is the subset of a pull request review the approval tracker needs.
type Review struct {
	User  string // reviewer login
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, ...
}
