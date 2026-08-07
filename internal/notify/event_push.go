package notify

import (
	"context"
	"strings"

	"github.com/google/go-github/v88/github"
)

type PushContext struct {
	Repo    string
	Ref     string
	RefType string // "branch" or "tag"
	Deleted bool   // true when this push deleted the ref (e.g. branch auto-deleted after a PR merge)
	Pusher  string
	Commits int
	Summary string
}

type pushHandler struct {
	destinations
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
	rawRef := e.GetRef()
	refType := "branch"
	if strings.HasPrefix(rawRef, "refs/tags/") {
		refType = "tag"
	}
	data := PushContext{
		Repo:    esc(e.GetRepo().GetFullName()),
		Ref:     esc(shortRef(rawRef)),
		RefType: refType,
		Deleted: e.GetDeleted(),
		Pusher:  esc(e.GetPusher().GetName()),
		Commits: len(e.Commits),
		Summary: summary,
	}
	telegramText, err := h.loader.render("push", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("push.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
