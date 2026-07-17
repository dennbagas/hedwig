package notify

import (
	"context"

	"hedwig/internal/telegrambot"

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
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *issueCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.IssueCommentEvent)
	if !ok {
		return nil
	}
	text, err := h.loader.render("issue_comment", IssueCommentContext{
		Action:   e.GetAction(),
		PRNumber: e.GetIssue().GetNumber(),
		PRTitle:  esc(e.GetIssue().GetTitle()),
		Author:   esc(e.GetSender().GetLogin()),
		Body:     esc(truncate(e.GetComment().GetBody(), 120)),
		URL:      esc(e.GetComment().GetHTMLURL()),
		IsPR:     e.GetIssue().GetPullRequestLinks() != nil,
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
