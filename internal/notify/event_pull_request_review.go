package notify

import (
	"context"

	"hedwig/internal/telegrambot"

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
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *pullRequestReviewHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewEvent)
	if !ok {
		return nil
	}
	text, err := h.loader.render("pull_request_review", PullRequestReviewContext{
		Action:   e.GetAction(),
		PRNumber: e.GetPullRequest().GetNumber(),
		PRTitle:  esc(e.GetPullRequest().GetTitle()),
		Reviewer: esc(e.GetReview().GetUser().GetLogin()),
		State:    reviewStateLabel(e.GetReview().GetState()),
		URL:      esc(e.GetReview().GetHTMLURL()),
	})
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	_, err = h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}
