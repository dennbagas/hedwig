package notify

import (
	"context"

	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

type PullRequestContext struct {
	Action   string
	Number   int
	Title    string
	Author   string
	MergedBy string // login of the user who merged; non-empty only when Merged is true
	Head     string
	Base     string
	URL      string
	Merged   bool
}

type pullRequestHandler struct {
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *pullRequestHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestEvent)
	if !ok {
		return nil
	}
	pr := e.GetPullRequest()
	text, err := h.loader.render("pull_request", PullRequestContext{
		Action:   e.GetAction(),
		Number:   pr.GetNumber(),
		Title:    esc(pr.GetTitle()),
		Author:   esc(pr.GetUser().GetLogin()),
		MergedBy: esc(pr.GetMergedBy().GetLogin()),
		Head:     esc(pr.GetHead().GetLabel()),
		Base:     esc(pr.GetBase().GetLabel()),
		URL:      esc(pr.GetHTMLURL()),
		Merged:   pr.GetMerged(),
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
