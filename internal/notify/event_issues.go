package notify

import (
	"context"
	"strings"

	"github.com/google/go-github/v88/github"
)

type IssuesContext struct {
	Action string
	Number int
	Title  string
	Body   string
	Author string
	Repo   string
	URL    string
	State  string
	Labels string
}

type issuesHandler struct {
	destinations
	loader *templateLoader
}

func (h *issuesHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.IssuesEvent)
	if !ok {
		return nil
	}
	issue := e.GetIssue()
	labels := make([]string, 0, len(issue.GetLabels()))
	for _, label := range issue.GetLabels() {
		labels = append(labels, label.GetName())
	}
	data := IssuesContext{
		Action: e.GetAction(),
		Number: issue.GetNumber(),
		Title:  esc(issue.GetTitle()),
		Body:   esc(truncate(issue.GetBody(), 120)),
		Author: esc(issue.GetUser().GetLogin()),
		Repo:   esc(e.GetRepo().GetFullName()),
		URL:    esc(issue.GetHTMLURL()),
		State:  esc(issue.GetState()),
		Labels: esc(strings.Join(labels, ", ")),
	}
	telegramText, err := h.loader.render("issues", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("issues.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return h.send(ctx, telegramText, slackText)
}
