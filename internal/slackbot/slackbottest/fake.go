// Package slackbottest provides a fake slackbot.Client for use in tests,
// following the same companion-package convention as net/http/httptest.
package slackbottest

import (
	"context"
	"strconv"
	"sync"

	"hedwig/internal/slackbot"
)

// SentMessage records a single PostMessage or UpdateMessage call.
type SentMessage struct {
	Channel string
	Ts      string // assigned by FakeClient for PostMessage; as given for UpdateMessage
	Text    string
	Buttons []slackbot.Button
	Updated bool
}

// FakeClient is an in-memory slackbot.Client test double. Every call is
// recorded for assertions; set the *Err fields to make a method return an
// error instead of succeeding. Safe for concurrent use.
type FakeClient struct {
	mu sync.Mutex

	nextTs int64

	Sent []SentMessage

	PostMessageErr   error
	UpdateMessageErr error
}

// New returns a ready-to-use FakeClient.
func New() *FakeClient {
	return &FakeClient{nextTs: 1}
}

func (f *FakeClient) PostMessage(_ context.Context, channel, text string, buttons []slackbot.Button) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PostMessageErr != nil {
		return "", f.PostMessageErr
	}
	ts := strconv.FormatInt(f.nextTs, 10) + ".000000"
	f.nextTs++
	f.Sent = append(f.Sent, SentMessage{
		Channel: channel,
		Ts:      ts,
		Text:    text,
		Buttons: buttons,
	})
	return ts, nil
}

func (f *FakeClient) UpdateMessage(_ context.Context, channel, ts, text string, buttons []slackbot.Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.UpdateMessageErr != nil {
		return f.UpdateMessageErr
	}
	f.Sent = append(f.Sent, SentMessage{
		Channel: channel,
		Ts:      ts,
		Text:    text,
		Buttons: buttons,
		Updated: true,
	})
	return nil
}

var _ slackbot.Client = (*FakeClient)(nil)
