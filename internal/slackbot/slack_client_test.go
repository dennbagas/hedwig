package slackbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

// newTestClient starts a fake Slack Web API server driven by handler and
// returns a slackClient pointed at it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *slackClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &slackClient{api: slack.New("test-token", slack.OptionAPIURL(server.URL+"/"))}
}

func writeOK(w http.ResponseWriter, extra string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true,"channel":"C1","ts":"1234.5678"`+extra+`}`)
}

func TestSlackClientPostMessage(t *testing.T) {
	var gotChannel, gotText string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotChannel = r.FormValue("channel")
		gotText = r.FormValue("text")
		writeOK(w, "")
	})

	ts, err := client.PostMessage(context.Background(), "C1", "hello world", nil)
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}
	if ts != "1234.5678" {
		t.Errorf("PostMessage() ts = %q, want 1234.5678", ts)
	}
	if gotChannel != "C1" {
		t.Errorf("channel = %q, want C1", gotChannel)
	}
	if gotText != "hello world" {
		t.Errorf("text = %q, want %q", gotText, "hello world")
	}
}

func TestSlackClientPostMessageWithButtons(t *testing.T) {
	var gotBlocks string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotBlocks = r.FormValue("blocks")
		writeOK(w, "")
	})

	_, err := client.PostMessage(context.Background(), "C1", "pick one", []Button{
		{Text: "Retry", Value: "hedwig:retry:trigger:1"},
	})
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}
	if !strings.Contains(gotBlocks, "hedwig:retry:trigger:1") {
		t.Errorf("blocks = %q, want it to contain the button value", gotBlocks)
	}
	if !strings.Contains(gotBlocks, "actions") {
		t.Errorf("blocks = %q, want an actions block", gotBlocks)
	}
}

func TestSlackClientUpdateMessage(t *testing.T) {
	var gotChannel, gotTs, gotText string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotChannel = r.FormValue("channel")
		gotTs = r.FormValue("ts")
		gotText = r.FormValue("text")
		writeOK(w, "")
	})

	if err := client.UpdateMessage(context.Background(), "C1", "1234.5678", "updated text", nil); err != nil {
		t.Fatalf("UpdateMessage() error = %v", err)
	}
	if gotChannel != "C1" || gotTs != "1234.5678" || gotText != "updated text" {
		t.Errorf("got (channel=%q, ts=%q, text=%q), want (C1, 1234.5678, updated text)", gotChannel, gotTs, gotText)
	}
}

func TestSlackClientUpdateMessageClearsButtons(t *testing.T) {
	var gotBlocks string
	var sawBlocksParam bool
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotBlocks = r.FormValue("blocks")
		_, sawBlocksParam = r.Form["blocks"]
		writeOK(w, "")
	})

	if err := client.UpdateMessage(context.Background(), "C1", "1234.5678", "no more buttons", nil); err != nil {
		t.Fatalf("UpdateMessage() error = %v", err)
	}
	if !sawBlocksParam {
		t.Fatal("expected the blocks param to be sent explicitly (empty), not omitted")
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal([]byte(gotBlocks), &blocks); err != nil {
		t.Fatalf("blocks param is not valid JSON: %v, got %q", err, gotBlocks)
	}
	if len(blocks) != 0 {
		t.Errorf("blocks = %q, want an empty array to clear buttons", gotBlocks)
	}
}

func TestSlackClientAPIErrorSurfaces(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":false,"error":"channel_not_found"}`)
	})

	_, err := client.PostMessage(context.Background(), "C1", "hi", nil)
	if err == nil {
		t.Fatal("PostMessage() error = nil, want an error surfaced from the Slack API response")
	}
	if !strings.Contains(err.Error(), "channel_not_found") {
		t.Errorf("PostMessage() error = %q, want it to include the API error", err.Error())
	}
}
