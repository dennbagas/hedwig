package notify

import (
	"context"

	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

type PullRequestReviewCommentContext struct {
	Action   string
	PRNumber int
	PRTitle  string
	Author   string
	File     string
	Line     int
	Body     string
	URL      string
}

type pullRequestReviewCommentHandler struct {
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *pullRequestReviewCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewCommentEvent)
	if !ok {
		return nil
	}
	text, err := h.loader.render("pull_request_review_comment", PullRequestReviewCommentContext{
		Action:   e.GetAction(),
		PRNumber: e.GetPullRequest().GetNumber(),
		PRTitle:  esc(e.GetPullRequest().GetTitle()),
		Author:   esc(e.GetSender().GetLogin()),
		File:     esc(e.GetComment().GetPath()),
		Line:     e.GetComment().GetLine(),
		Body:     esc(truncate(e.GetComment().GetBody(), 120)),
		URL:      esc(e.GetComment().GetHTMLURL()),
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
