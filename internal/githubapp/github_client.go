package githubapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v66/github"
)

type githubClient struct {
	client        *github.Client
	webhookSecret string
}

func New(httpClient *http.Client, webhookSecret string) Client {
	return &githubClient{
		client:        github.NewClient(httpClient),
		webhookSecret: webhookSecret,
	}
}

func (c *githubClient) ValidateWebhook(r *http.Request) ([]byte, error) {
	payload, err := github.ValidatePayload(r, []byte(c.webhookSecret))
	if err != nil {
		return nil, fmt.Errorf("validate webhook signature: %w", err)
	}
	return payload, nil
}

func (c *githubClient) ParseWebhook(eventType string, payload []byte) (interface{}, error) {
	return github.ParseWebHook(eventType, payload)
}

func (c *githubClient) RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error {
	_, err := c.client.Actions.RerunFailedJobsByID(ctx, owner, repo, runID)
	return err
}

func (c *githubClient) CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (string, error) {
	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String(title),
		Body:  github.String(body),
		Head:  github.String(head),
		Base:  github.String(base),
	})
	if err != nil {
		return "", fmt.Errorf("create pull request: %w", parsePRError(err, owner, repo, head, base))
	}
	return pr.GetHTMLURL(), nil
}

func parsePRError(err error, owner, repo, head, base string) error {
	ghErr, ok := err.(*github.ErrorResponse)
	if !ok {
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
