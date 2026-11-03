package notify

import (
	"context"
	"fmt"
	"strings"

	"hedwig/internal/approval"
)

// ApprovalRole is one role's progress, as exposed to the approval template.
type ApprovalRole struct {
	Role string
	Have int
	Need int
	Met  bool
}

// ApprovalContext is the template data for the "approval" status message.
// String fields are HTML-escaped (for the Telegram template); the Slack
// template uses the same fields as plain mrkdwn.
type ApprovalContext struct {
	PRNumber         int
	Base             string
	Repo             string // "owner/repo", used to build the repository link
	Title            string
	URL              string
	Satisfied        bool
	Roles            []ApprovalRole
	Missing          string // e.g. "1 developer, 1 infra"; empty when satisfied
	ChangesRequested string // comma-joined reviewer logins; empty when none
}

// sendApprovalStatus renders the approval status templates from res and the
// PR's display fields, then delivers them to whichever platforms are enabled.
// Called on each review change so the chat shows running progress.
func sendApprovalStatus(ctx context.Context, dest destinations, loader *templateLoader, res *approval.Result, prNumber int, base, repo, title, url string) error {
	data := ApprovalContext{
		PRNumber:         prNumber,
		Base:             esc(base),
		Repo:             esc(repo),
		Title:            esc(title),
		URL:              esc(url),
		Satisfied:        res.Satisfied,
		Missing:          missingText(res.Roles),
		ChangesRequested: esc(strings.Join(res.ChangesRequested, ", ")),
	}
	for _, r := range res.Roles {
		data.Roles = append(data.Roles, ApprovalRole{
			Role: esc(r.Role),
			Have: r.Have,
			Need: r.Need,
			Met:  r.Met(),
		})
	}

	telegramText, err := loader.render("approval", data)
	if err != nil {
		return err
	}
	slackText, err := loader.render("approval.slack", data)
	if err != nil {
		return err
	}
	if telegramText == "" && slackText == "" {
		return nil
	}
	return dest.send(ctx, telegramText, slackText)
}

// missingText summarizes the shortfall across unmet roles, e.g. a role needing
// 1 of 2 and another needing 1 of 1 yields "1 developer, 1 infra".
func missingText(roles []approval.RoleStatus) string {
	var parts []string
	for _, r := range roles {
		if !r.Met() {
			parts = append(parts, fmt.Sprintf("%d %s", r.Need-r.Have, r.Role))
		}
	}
	return strings.Join(parts, ", ")
}
