package notify

import (
	"context"

	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

type CreateContext struct {
	RefType string
	Ref     string
	Repo    string
	Creator string
	URL     string
}

type createHandler struct {
	tg     telegrambot.Client
	chatID int64
	loader *templateLoader
}

func (h *createHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.CreateEvent)
	if !ok {
		return nil
	}
	repo := e.GetRepo().GetFullName()
	ref := e.GetRef()
	var url string
	if e.GetRefType() == "tag" {
		url = "https://github.com/" + repo + "/releases/tag/" + ref
	} else {
		url = "https://github.com/" + repo + "/tree/" + ref
	}
	text, err := h.loader.render("create", CreateContext{
		RefType: e.GetRefType(),
		Ref:     esc(ref),
		Repo:    esc(repo),
		Creator: esc(e.GetSender().GetLogin()),
		URL:     esc(url),
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
