// Package githubapptest provides a fake githubapp.Client for use in tests,
// following the same companion-package convention as net/http/httptest.
package githubapptest

import (
	"context"
	"net/http"
	"sync"

	"github.com/btse/hedwig/internal/githubapp"
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

var _ githubapp.Client = (*FakeClient)(nil)
