package notify

import (
	"context"
	"errors"

	"hedwig/internal/approval"

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
	// approvalH evaluates the branch approval gate; nil when the gate is
	// disabled (approval.enabled=false), in which case it is skipped.
	approvalH *approval.Handler
}

func (h *pullRequestHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestEvent)
	if !ok {
		return nil
	}
	pr := e.GetPullRequest()

	// Send the PR lifecycle notification (e.g. "New Pull Request") first, then
	// the approval status below it, so the chat reads top-to-bottom in event
	// order (Telegram orders by send time).
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
	var sendErr error
	if telegramText != "" || slackText != "" {
		sendErr = h.send(ctx, telegramText, slackText)
	}

	// Re-evaluate the approval gate whenever the PR's commits or review scope
	// change. Other actions (labeled, assigned, closed, ...) don't affect the
	// approval count, so they're skipped to avoid needless API calls.
	var approvalErr error
	if h.approvalH != nil {
		switch e.GetAction() {
		case "opened", "reopened", "synchronize", "ready_for_review", "edited":
			res, err := h.approvalH.Evaluate(ctx, approval.PR{
				Owner:   e.GetRepo().GetOwner().GetLogin(),
				OwnerID: e.GetRepo().GetOwner().GetID(),
				Repo:    e.GetRepo().GetName(),
				Base:    pr.GetBase().GetRef(),
				Number:  pr.GetNumber(),
			})
			approvalErr = err
			// Post the status to chat on PR lifecycle changes, but stay quiet
			// on each push (synchronize) and metadata edit — those still
			// re-evaluate, just without spamming the channel.
			if err == nil && res != nil {
				switch e.GetAction() {
				case "opened", "reopened", "ready_for_review":
					approvalErr = sendApprovalStatus(ctx, h.destinations, h.loader, res,
						pr.GetNumber(), pr.GetBase().GetRef(), e.GetRepo().GetFullName(), pr.GetTitle(), pr.GetHTMLURL())
				}
			}
		}
	}

	// Both the notification and the gate are attempted; their errors are
	// joined so a failure in either surfaces to Dispatch (and triggers a
	// GitHub webhook redelivery), without one suppressing the other.
	return errors.Join(sendErr, approvalErr)
}
