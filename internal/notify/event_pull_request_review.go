package notify

import (
	"context"
	"errors"

	"hedwig/internal/approval"

	"github.com/google/go-github/v88/github"
)

type PullRequestReviewContext struct {
	Action   string
	PRNumber int
	PRTitle  string
	Reviewer string
	State    string
	URL      string
}

type pullRequestReviewHandler struct {
	destinations
	loader *templateLoader
	// approvalH evaluates the branch approval gate; nil when disabled.
	approvalH *approval.Handler
}

func (h *pullRequestReviewHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewEvent)
	if !ok {
		return nil
	}

	// Send the per-review notification first, then the approval status, so the
	// chat shows "PR Review — Approved" above the resulting "Approval ..."
	// summary (Telegram orders by send time).
	data := PullRequestReviewContext{
		Action:   e.GetAction(),
		PRNumber: e.GetPullRequest().GetNumber(),
		PRTitle:  esc(e.GetPullRequest().GetTitle()),
		Reviewer: esc(e.GetReview().GetUser().GetLogin()),
		State:    reviewStateLabel(e.GetReview().GetState()),
		URL:      esc(e.GetReview().GetHTMLURL()),
	}
	telegramText, err := h.loader.render("pull_request_review", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("pull_request_review.slack", data)
	if err != nil {
		return err
	}
	var sendErr error
	if telegramText != "" || slackText != "" {
		sendErr = h.send(ctx, telegramText, slackText)
	}

	// A submitted, edited, or dismissed review changes the approval tally, so
	// re-evaluate the gate and post running progress to chat for each one.
	var approvalErr error
	if h.approvalH != nil {
		switch e.GetAction() {
		case "submitted", "edited", "dismissed":
			pr := e.GetPullRequest()
			res, err := h.approvalH.Evaluate(ctx, approval.PR{
				Owner:   e.GetRepo().GetOwner().GetLogin(),
				OwnerID: e.GetRepo().GetOwner().GetID(),
				Repo:    e.GetRepo().GetName(),
				Base:    pr.GetBase().GetRef(),
				Number:  pr.GetNumber(),
			})
			approvalErr = err
			if err == nil && res != nil {
				approvalErr = sendApprovalStatus(ctx, h.destinations, h.loader, res,
					pr.GetNumber(), pr.GetBase().GetRef(), e.GetRepo().GetFullName(), pr.GetTitle(), pr.GetHTMLURL())
			}
		}
	}

	return errors.Join(sendErr, approvalErr)
}
