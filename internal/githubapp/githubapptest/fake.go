// Package githubapptest provides a fake githubapp.Client for use in tests,
// following the same companion-package convention as net/http/httptest.
package githubapptest

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"hedwig/internal/githubapp"
)

// RerunCall records a single RerunFailedJobs call.
type RerunCall struct {
	Owner, Repo string
	RunID       int64
}

// CreatePRCall records a single CreatePR call.
type CreatePRCall struct {
	Owner, Repo, Title, Body, Head, Base string
}

// FakeClient is an in-memory githubapp.Client test double. Every call is
// recorded for assertions. ValidateWebhook/ParseWebhook behavior is
// request/payload-dependent, so they're driven by injectable func fields
// (defaulting to an "always valid, empty event" no-op); RerunFailedJobs and
// CreatePR are driven by fixed injectable results/errors. Safe for
// concurrent use.
type FakeClient struct {
	mu sync.Mutex

	ValidateWebhookFunc func(r *http.Request) ([]byte, error)
	ParseWebhookFunc    func(eventType string, payload []byte) (any, error)

	RerunFailedJobsErr error
	CreatePRResult     string
	CreatePRErr        error

	// Reviews returned by ListPullRequestReviews, and an optional error.
	Reviews        []githubapp.Review
	ListReviewsErr error
	// TeamMembers maps "teamSlug/username" to active membership. Any pair not
	// present is treated as a non-member (false).
	TeamMembers     map[string]bool
	IsTeamMemberErr error
	// BranchRules is returned by GetBranchApprovalRules, with an optional error.
	BranchRules    githubapp.BranchApprovalRules
	BranchRulesErr error
	// TeamSlugs maps team ID to slug for GetTeamSlug; a missing ID is an error.
	TeamSlugs map[int64]string

	RerunCalls    []RerunCall
	CreatePRCalls []CreatePRCall
}

// New returns a ready-to-use FakeClient.
func New() *FakeClient {
	return &FakeClient{}
}

func (f *FakeClient) ValidateWebhook(r *http.Request) ([]byte, error) {
	if f.ValidateWebhookFunc != nil {
		return f.ValidateWebhookFunc(r)
	}
	return nil, nil
}

func (f *FakeClient) ParseWebhook(eventType string, payload []byte) (any, error) {
	if f.ParseWebhookFunc != nil {
		return f.ParseWebhookFunc(eventType, payload)
	}
	return nil, nil
}

func (f *FakeClient) RerunFailedJobs(_ context.Context, owner, repo string, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RerunCalls = append(f.RerunCalls, RerunCall{Owner: owner, Repo: repo, RunID: runID})
	return f.RerunFailedJobsErr
}

func (f *FakeClient) CreatePR(_ context.Context, owner, repo, title, body, head, base string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreatePRCalls = append(f.CreatePRCalls, CreatePRCall{Owner: owner, Repo: repo, Title: title, Body: body, Head: head, Base: base})
	if f.CreatePRErr != nil {
		return "", f.CreatePRErr
	}
	return f.CreatePRResult, nil
}

func (f *FakeClient) ListPullRequestReviews(_ context.Context, _, _ string, _ int) ([]githubapp.Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListReviewsErr != nil {
		return nil, f.ListReviewsErr
	}
	return f.Reviews, nil
}

func (f *FakeClient) IsTeamMember(_ context.Context, _, teamSlug, username string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.IsTeamMemberErr != nil {
		return false, f.IsTeamMemberErr
	}
	return f.TeamMembers[teamSlug+"/"+username], nil
}

func (f *FakeClient) GetBranchApprovalRules(_ context.Context, _, _, _ string) (githubapp.BranchApprovalRules, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.BranchRules, f.BranchRulesErr
}

func (f *FakeClient) GetTeamSlug(_ context.Context, _, teamID int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	slug, ok := f.TeamSlugs[teamID]
	if !ok {
		return "", fmt.Errorf("fake: unknown team id %d", teamID)
	}
	return slug, nil
}

var _ githubapp.Client = (*FakeClient)(nil)
