package notify

import (
	"context"

	"github.com/google/go-github/v88/github"
)

type ReleaseContext struct {
	Action     string // "published", "edited", etc.
	TagName    string
	Name       string // release title
	Body       string // release notes
	Author     string
	Repo       string
	URL        string
	Prerelease bool
}

type releaseHandler struct {
	destinations
	loader *templateLoader
}

func (h *releaseHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.ReleaseEvent)
	if !ok {
		return nil
	}
	rel := e.GetRelease()
	data := ReleaseContext{
		Action:     e.GetAction(),
		TagName:    esc(rel.GetTagName()),
		Name:       esc(rel.GetName()),
		Body:       esc(rel.GetBody()),
		Author:     esc(rel.GetAuthor().GetLogin()),
		Repo:       esc(e.GetRepo().GetFullName()),
		URL:        esc(rel.GetHTMLURL()),
		Prerelease: rel.GetPrerelease(),
	}
	telegramText, err := h.loader.render("release", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("release.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
