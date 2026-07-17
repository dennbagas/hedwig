package notify

import (
	"context"

	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

type PushContext struct {
	Repo    string
	Ref     string
	Pusher  string
	Commits int
	Summary string
}

type pushHandler struct {
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *pushHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PushEvent)
	if !ok {
		return nil
	}
	summary := ""
	if e.GetHeadCommit() != nil {
		summary = esc(truncate(e.GetHeadCommit().GetMessage(), 80))
	}
	text, err := h.loader.render("push", PushContext{
		Repo:    esc(e.GetRepo().GetFullName()),
		Ref:     esc(shortRef(e.GetRef())),
		Pusher:  esc(e.GetPusher().GetName()),
		Commits: len(e.Commits),
		Summary: summary,
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
