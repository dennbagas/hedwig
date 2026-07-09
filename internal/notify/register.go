package notify

import (
	"github.com/btse/hedwig/internal/retry"
	"github.com/btse/hedwig/internal/telegrambot"
)

// RegisterAll wires all event handlers onto d.
func RegisterAll(d *Dispatcher, tg telegrambot.Client, retryH *retry.Handler, chatID int64) {
	d.Register("push", &pushHandler{tg: tg, chatID: chatID})
	d.Register("pull_request", &pullRequestHandler{tg: tg, chatID: chatID})
	d.Register("create", &createHandler{tg: tg, chatID: chatID})
	d.Register("issue_comment", &issueCommentHandler{tg: tg, chatID: chatID})
	d.Register("pull_request_review", &pullRequestReviewHandler{tg: tg, chatID: chatID})
	d.Register("pull_request_review_comment", &pullRequestReviewCommentHandler{tg: tg, chatID: chatID})
	d.Register("workflow_run", &workflowRunHandler{tg: tg, chatID: chatID, retryH: retryH})
}
