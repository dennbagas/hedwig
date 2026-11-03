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
}

// Review is the subset of a pull request review the approval tracker needs.
type Review struct {
	User  string // reviewer login
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, ...
}
