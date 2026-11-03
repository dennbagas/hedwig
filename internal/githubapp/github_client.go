package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v88/github"
)

type githubClient struct {
	client        *github.Client
	webhookSecret string
}

func New(httpClient *http.Client, webhookSecret string) (Client, error) {
	client, err := github.NewClient(github.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}
	return &githubClient{
		client:        client,
		webhookSecret: webhookSecret,
	}, nil
}

func (c *githubClient) ValidateWebhook(r *http.Request) ([]byte, error) {
	payload, err := github.ValidatePayload(r, []byte(c.webhookSecret))
	if err != nil {
		return nil, fmt.Errorf("validate webhook signature: %w", err)
	}
	return payload, nil
}

func (c *githubClient) ParseWebhook(eventType string, payload []byte) (any, error) {
	return github.ParseWebHook(eventType, payload)
}

func (c *githubClient) RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error {
	_, err := c.client.Actions.RerunFailedJobsByID(ctx, owner, repo, runID)
	return err
}

func (c *githubClient) CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (string, error) {
	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: new(title),
		Body:  new(body),
		Head:  new(head),
		Base:  new(base),
	})
	if err != nil {
		return "", fmt.Errorf("create pull request: %w", parsePRError(err, owner, repo, head, base))
	}
	return pr.GetHTMLURL(), nil
}

func (c *githubClient) ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	var reviews []Review
	opt := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := c.client.PullRequests.ListReviews(ctx, owner, repo, number, opt)
		if err != nil {
			return nil, fmt.Errorf("list pull request reviews: %w", err)
		}
		for _, r := range page {
			reviews = append(reviews, Review{
				User:  r.GetUser().GetLogin(),
				State: r.GetState(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return reviews, nil
}

func (c *githubClient) IsTeamMember(ctx context.Context, org, teamSlug, username string) (bool, error) {
	membership, resp, err := c.client.Teams.GetTeamMembershipBySlug(ctx, org, teamSlug, username)
	if err != nil {
		// A user who isn't on the team yields 404 — a normal "no", not a failure.
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("get team membership %s/%s for %s: %w", org, teamSlug, username, err)
	}
	// Pending invitations report state "pending"; only "active" counts.
	return membership.GetState() == "active", nil
}

func parsePRError(err error, owner, repo, head, base string) error {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) {
		return err
	}

	switch ghErr.Response.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("repository %s/%s not found or no access", owner, repo)
	case http.StatusUnprocessableEntity:
		for _, e := range ghErr.Errors {
			if e.Code == "custom" && e.Message != "" {
				return fmt.Errorf("%s", e.Message)
			}
			switch e.Field {
			case "head":
				return fmt.Errorf("source branch %q not found in %s/%s", head, owner, repo)
			case "base":
				return fmt.Errorf("target branch %q not found in %s/%s", base, owner, repo)
			}
		}
	}
	return err
}
