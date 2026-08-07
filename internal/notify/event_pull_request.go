package notify

import (
	"context"

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
	Repo     string
	URL      string
	Merged   bool
}

type pullRequestHandler struct {
	destinations
	loader *templateLoader
}

func (h *pullRequestHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestEvent)
	if !ok {
		return nil
	}
	pr := e.GetPullRequest()
	data := PullRequestContext{
		Action:   e.GetAction(),
		Number:   pr.GetNumber(),
		Title:    esc(pr.GetTitle()),
		Author:   esc(pr.GetUser().GetLogin()),
		MergedBy: esc(pr.GetMergedBy().GetLogin()),
		Head:     esc(pr.GetHead().GetLabel()),
		Base:     esc(pr.GetBase().GetLabel()),
		Repo:     esc(e.GetRepo().GetFullName()),
		URL:      esc(pr.GetHTMLURL()),
		Merged:   pr.GetMerged(),
	}
	telegramText, err := h.loader.render("pull_request", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("pull_request.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
