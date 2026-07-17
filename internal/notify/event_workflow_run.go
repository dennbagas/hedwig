package notify

import (
	"context"

	"hedwig/internal/retry"
	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
)

type WorkflowRunContext struct {
	Action     string
	Name       string
	Repo       string
	Branch     string
	HeadSHA    string // first 7 chars of the triggering commit SHA
	Conclusion string
	URL        string
	RunID      int64
}

// workflowRunHandler handles workflow_run events using a configurable template.
// On completed+failure it delegates to retry.Handler which attaches the retry button.
type workflowRunHandler struct {
	tg     telegrambot.Client
	chatID int64
	retryH *retry.Handler
	loader *templateLoader
}

func (h *workflowRunHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.WorkflowRunEvent)
	if !ok {
		return nil
	}
	text, err := h.loader.render("workflow_run", WorkflowRunContext{
		Action:     e.GetAction(),
		Name:       esc(e.GetWorkflowRun().GetName()),
		Repo:       esc(e.GetRepo().GetFullName()),
		Branch:     esc(e.GetWorkflowRun().GetHeadBranch()),
		HeadSHA:    shortSHA(e.GetWorkflowRun().GetHeadSHA()),
		Conclusion: e.GetWorkflowRun().GetConclusion(),
		URL:        esc(e.GetWorkflowRun().GetHTMLURL()),
		RunID:      e.GetWorkflowRun().GetID(),
	})
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	if e.GetAction() == "completed" && e.GetWorkflowRun().GetConclusion() == "failure" {
		return h.retryH.NotifyFailure(ctx, h.chatID,
			e.GetWorkflowRun().GetName(),
			e.GetRepo().GetOwner().GetLogin(),
			e.GetRepo().GetName(),
			e.GetWorkflowRun().GetID(),
			text,
		)
	}
	_, err = h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}
