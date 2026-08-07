package slackbot

import (
	"context"
)

// Button is one interactive button attached to a message, analogous to
// telegrambot.Button. Value is the encoded callback payload — for the CI/CD
// retry button this reuses telegrambot.EncodeCallback's
// "hedwig:<feature>:<action>:<payload>" format so both platforms decode the
// same way.
type Button struct {
	Text  string
	Value string
}

// Client is a thin interface over the Slack Web API, mirroring
// telegrambot.Client's shape so notify handlers and retry.Handler can treat
// both platforms uniformly at the call site.
type Client interface {
	// PostMessage sends text to channel, optionally with a row of buttons,
	// and returns the message's ts (Slack's string timestamp message ID).
	PostMessage(ctx context.Context, channel, text string, buttons []Button) (ts string, err error)
	// UpdateMessage edits an existing message in place. Pass buttons=nil to
	// strip any existing buttons — Slack has no separate "clear markup"
	// call; chat.update with blocks explicitly emptied does both jobs.
	UpdateMessage(ctx context.Context, channel, ts, text string, buttons []Button) error
}
