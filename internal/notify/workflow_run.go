package notify

import (
	"context"

	"hedwig/internal/retry"
	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

// workflowRunHandler handles both "requested" and "completed" workflow_run events.
// On completion with failure it delegates to retry.Handler to send the message + button.
type workflowRunHandler struct {
	tg     telegrambot.Client
	chatID int64
	retryH *retry.Handler
}

func (h *workflowRunHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.WorkflowRunEvent)
	if !ok {
		return nil
	}

	switch e.GetAction() {
	case "requested":
		name := e.GetWorkflowRun().GetName()
		repo := e.GetRepo().GetFullName()
		branch := e.GetWorkflowRun().GetHeadBranch()
		url := e.GetWorkflowRun().GetHTMLURL()
		text := "CI/CD started: <b>" + esc(name) + "</b>\n" + esc(repo) + " on " + esc(branch) + "\n" + htmlLink("View run", url)
		_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
		return err

	case "completed":
		conclusion := e.GetWorkflowRun().GetConclusion()
		if conclusion != "failure" {
			name := e.GetWorkflowRun().GetName()
			repo := e.GetRepo().GetFullName()
			url := e.GetWorkflowRun().GetHTMLURL()
			text := "CI/CD " + esc(conclusion) + ": <b>" + esc(name) + "</b>\n" + esc(repo) + "\n" + htmlLink("View run", url)
			_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
			return err
		}
		// Failure: delegate to retry handler which sends the message + retry button.
		return h.retryH.NotifyFailure(ctx, h.chatID,
			e.GetWorkflowRun().GetName(),
			e.GetRepo().GetOwner().GetLogin(),
			e.GetRepo().GetName(),
			e.GetWorkflowRun().GetID(),
			e.GetWorkflowRun().GetHTMLURL(),
		)
	}
	return nil
}
