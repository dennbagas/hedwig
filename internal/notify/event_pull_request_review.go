package notify

import (
	"context"

	"github.com/google/go-github/v88/github"
)

type PullRequestReviewContext struct {
	Action   string
	PRNumber int
	PRTitle  string
	Reviewer string
	State    string
	URL      string
}

type pullRequestReviewHandler struct {
	destinations
	loader *templateLoader
}

func (h *pullRequestReviewHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewEvent)
	if !ok {
		return nil
	}
	data := PullRequestReviewContext{
		Action:   e.GetAction(),
		PRNumber: e.GetPullRequest().GetNumber(),
		PRTitle:  esc(e.GetPullRequest().GetTitle()),
		Reviewer: esc(e.GetReview().GetUser().GetLogin()),
		State:    reviewStateLabel(e.GetReview().GetState()),
		URL:      esc(e.GetReview().GetHTMLURL()),
	}
	telegramText, err := h.loader.render("pull_request_review", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("pull_request_review.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
