package notify

import (
	"context"

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
	destinations
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
	data := CreateContext{
		RefType: e.GetRefType(),
		Ref:     esc(ref),
		Repo:    esc(repo),
		Creator: esc(e.GetSender().GetLogin()),
		URL:     esc(url),
	}
	telegramText, err := h.loader.render("create", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("create.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
