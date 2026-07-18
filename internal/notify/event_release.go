package notify

import (
	"context"

	"hedwig/internal/telegrambot"

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
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *releaseHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.ReleaseEvent)
	if !ok {
		return nil
	}
	rel := e.GetRelease()
	text, err := h.loader.render("release", ReleaseContext{
		Action:     e.GetAction(),
		TagName:    esc(rel.GetTagName()),
		Name:       esc(rel.GetName()),
		Body:       esc(rel.GetBody()),
		Author:     esc(rel.GetAuthor().GetLogin()),
		Repo:       esc(e.GetRepo().GetFullName()),
		URL:        esc(rel.GetHTMLURL()),
		Prerelease: rel.GetPrerelease(),
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
