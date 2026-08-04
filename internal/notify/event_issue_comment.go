package notify

import (
	"context"

	"github.com/google/go-github/v88/github"
)

type IssueCommentContext struct {
	Action   string
	PRNumber int
	PRTitle  string
	Author   string
	Body     string
	URL      string
	IsPR     bool
}

type issueCommentHandler struct {
	destinations
	loader *templateLoader
}

func (h *issueCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.IssueCommentEvent)
	if !ok {
		return nil
	}
	data := IssueCommentContext{
		Action:   e.GetAction(),
		PRNumber: e.GetIssue().GetNumber(),
		PRTitle:  esc(e.GetIssue().GetTitle()),
		Author:   esc(e.GetSender().GetLogin()),
		Body:     esc(truncate(e.GetComment().GetBody(), 120)),
		URL:      esc(e.GetComment().GetHTMLURL()),
		IsPR:     e.GetIssue().GetPullRequestLinks() != nil,
	}
	telegramText, err := h.loader.render("issue_comment", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("issue_comment.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
