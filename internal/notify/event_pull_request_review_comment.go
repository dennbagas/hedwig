package notify

import (
	"context"

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
	destinations
	loader *templateLoader
}

func (h *pullRequestReviewCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewCommentEvent)
	if !ok {
		return nil
	}
	data := PullRequestReviewCommentContext{
		Action:   e.GetAction(),
		PRNumber: e.GetPullRequest().GetNumber(),
		PRTitle:  esc(e.GetPullRequest().GetTitle()),
		Author:   esc(e.GetSender().GetLogin()),
		File:     esc(e.GetComment().GetPath()),
		Line:     e.GetComment().GetLine(),
		Body:     esc(truncate(e.GetComment().GetBody(), 120)),
		URL:      esc(e.GetComment().GetHTMLURL()),
	}
	telegramText, err := h.loader.render("pull_request_review_comment", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("pull_request_review_comment.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
