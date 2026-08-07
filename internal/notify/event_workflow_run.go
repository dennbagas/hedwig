package notify

import (
	"context"

	"hedwig/internal/retry"

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

// workflowRunHandler handles workflow_run events using configurable
// templates. On completed+failure it delegates to retry.Handler, which
// attaches the retry button and posts to whichever platforms are enabled
// itself (see retry.Handler.NotifyFailure) — this handler renders both
// platforms' text but does not send it directly in that case.
type workflowRunHandler struct {
	destinations
	retryH *retry.Handler
	loader *templateLoader
}

func (h *workflowRunHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.WorkflowRunEvent)
	if !ok {
		return nil
	}
	data := WorkflowRunContext{
		Action:     e.GetAction(),
		Name:       esc(e.GetWorkflowRun().GetName()),
		Repo:       esc(e.GetRepo().GetFullName()),
		Branch:     esc(e.GetWorkflowRun().GetHeadBranch()),
		HeadSHA:    shortSHA(e.GetWorkflowRun().GetHeadSHA()),
		Conclusion: e.GetWorkflowRun().GetConclusion(),
		URL:        esc(e.GetWorkflowRun().GetHTMLURL()),
		RunID:      e.GetWorkflowRun().GetID(),
	}
	telegramText, err := h.loader.render("workflow_run", data)
	if err != nil {
		return err
	}
	slackText, err := h.loader.render("workflow_run.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	if e.GetAction() == "completed" && e.GetWorkflowRun().GetConclusion() == "failure" {
		return h.retryH.NotifyFailure(ctx, h.chatID,
			e.GetWorkflowRun().GetName(),
			e.GetRepo().GetOwner().GetLogin(),
			e.GetRepo().GetName(),
			e.GetWorkflowRun().GetID(),
			retry.FailureText{Telegram: telegramText, Slack: slackText},
		)
	}
	return h.send(ctx, telegramText, slackText)
}
