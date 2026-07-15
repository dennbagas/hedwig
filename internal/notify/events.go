package notify

import (
	"context"
	"fmt"

	"hedwig/internal/telegrambot"
	"github.com/google/go-github/v88/github"
)

// pushHandler handles push events.
type pushHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *pushHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PushEvent)
	if !ok {
		return nil
	}
	pusher := e.GetPusher().GetName()
	ref := shortRef(e.GetRef())
	repo := e.GetRepo().GetFullName()
	nCommits := len(e.Commits)
	summary := ""
	if e.GetHeadCommit() != nil {
		summary = truncate(e.GetHeadCommit().GetMessage(), 80)
	}
	text := fmt.Sprintf("Push to <b>%s/%s</b> by <b>%s</b>\n%d commit(s)\n%s",
		esc(repo), esc(ref), esc(pusher), nCommits, esc(summary))
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}

// pullRequestHandler handles PR opened and closed events.
type pullRequestHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *pullRequestHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestEvent)
	if !ok {
		return nil
	}
	action := e.GetAction()
	if action != "opened" && action != "closed" {
		return nil
	}
	pr := e.GetPullRequest()
	title := pr.GetTitle()
	author := pr.GetUser().GetLogin()
	head := pr.GetHead().GetLabel()
	base := pr.GetBase().GetLabel()
	url := pr.GetHTMLURL()
	var text string
	if action == "opened" {
		text = fmt.Sprintf("PR opened: %s\nBy <b>%s</b>\n%s → %s\n%s",
			htmlLink(title, url), esc(author), esc(head), esc(base), esc(url))
	} else {
		merged := pr.GetMerged()
		state := "closed without merge"
		if merged {
			state = "merged"
		}
		text = fmt.Sprintf("PR %s: %s\n%s", state, htmlLink(title, url), esc(url))
	}
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}

// createHandler handles tag and branch creation events.
type createHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *createHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.CreateEvent)
	if !ok {
		return nil
	}
	refType := e.GetRefType()
	if refType != "tag" && refType != "branch" {
		return nil
	}
	repo := e.GetRepo().GetFullName()
	ref := e.GetRef()
	pusher := e.GetSender().GetLogin()
	url := fmt.Sprintf("https://github.com/%s/tree/%s", repo, ref)
	text := fmt.Sprintf("%s created: %s in <b>%s</b> by <b>%s</b>\n%s",
		capitalize(refType), esc(ref), esc(repo), esc(pusher), htmlLink(ref, url))
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// issueCommentHandler handles PR comments posted via the issue_comment event.
type issueCommentHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *issueCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.IssueCommentEvent)
	if !ok {
		return nil
	}
	if e.GetAction() != "created" {
		return nil
	}
	if e.GetIssue().GetPullRequestLinks() == nil {
		return nil
	}
	commenter := e.GetSender().GetLogin()
	prTitle := e.GetIssue().GetTitle()
	excerpt := truncate(e.GetComment().GetBody(), 120)
	url := e.GetComment().GetHTMLURL()
	text := fmt.Sprintf("Comment on PR <b>%s</b> by <b>%s</b>\n%s\n%s",
		esc(prTitle), esc(commenter), esc(excerpt), htmlLink("View comment", url))
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}

// pullRequestReviewHandler handles PR review submissions.
type pullRequestReviewHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *pullRequestReviewHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewEvent)
	if !ok {
		return nil
	}
	if e.GetAction() != "submitted" {
		return nil
	}
	reviewer := e.GetReview().GetUser().GetLogin()
	prTitle := e.GetPullRequest().GetTitle()
	state := reviewStateLabel(e.GetReview().GetState())
	url := e.GetReview().GetHTMLURL()
	text := fmt.Sprintf("Review on PR <b>%s</b> by <b>%s</b>: %s\n%s",
		esc(prTitle), esc(reviewer), esc(state), htmlLink("View review", url))
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}

// pullRequestReviewCommentHandler handles inline review comments.
type pullRequestReviewCommentHandler struct {
	tg     telegrambot.Client
	chatID int64
}

func (h *pullRequestReviewCommentHandler) Handle(ctx context.Context, event any) error {
	e, ok := event.(*github.PullRequestReviewCommentEvent)
	if !ok {
		return nil
	}
	if e.GetAction() != "created" {
		return nil
	}
	commenter := e.GetSender().GetLogin()
	prTitle := e.GetPullRequest().GetTitle()
	file := e.GetComment().GetPath()
	line := e.GetComment().GetLine()
	excerpt := truncate(e.GetComment().GetBody(), 120)
	url := e.GetComment().GetHTMLURL()
	text := fmt.Sprintf("Review comment on PR <b>%s</b> by <b>%s</b>\n%s:%d\n%s\n%s",
		esc(prTitle), esc(commenter), esc(file), line, esc(excerpt), htmlLink("View comment", url))
	_, err := h.tg.SendMessage(ctx, h.chatID, text, telegrambot.WithParseMode("HTML"))
	return err
}
