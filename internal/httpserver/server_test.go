package httpserver

import (
	"os"
	"path/filepath"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/githubapp/githubapptest"
	"hedwig/internal/notify"
	"hedwig/internal/retry"
	"hedwig/internal/slackbot/slackbottest"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/rs/zerolog"
)

const testSlackSigningSecret = "test-slack-signing-secret"

// testServer wires a real *Server to a real notify.Dispatcher and
// retry.Handler, backed by fake GitHub/Telegram/Slack clients and a real
// temp-file SQLite repository — only the true external network edges are
// faked, so these tests exercise the actual routing/business logic. Slack
// is always enabled (its webhook route registered) so slack_webhook_test.go
// can exercise it directly; tests that don't care about Slack simply never
// touch ts.slack.
type testServer struct {
	*Server
	tg    *telegrambottest.FakeClient
	slack *slackbottest.FakeClient
	gh    *githubapptest.FakeClient
	store database.Repository
}

func newTestServer(t *testing.T, telegramSecret string) *testServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	store := database.NewSQLiteRepository(db)

	tg := telegrambottest.New()
	slack := slackbottest.New()
	gh := githubapptest.New()

	// Write a minimal push template so GitHub webhook tests get a real notification.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "push.tmpl"), []byte(`{{.Repo}} {{.Pusher}}`), 0o600); err != nil {
		t.Fatalf("write push template: %v", err)
	}

	retryH := retry.New(store, tg, slack, "C1", gh, zerolog.Nop())
	notifyD, err := notify.New(tg, 999, slack, "C1", retryH, tmpDir, zerolog.Nop())
	if err != nil {
		t.Fatalf("notify.New() error = %v", err)
	}

	srv := New(gh, store, notifyD, retryH, telegramSecret, "/healthz", "/webhooks/telegram",
		true, testSlackSigningSecret, "/webhooks/slack/interactions", zerolog.Nop())

	return &testServer{Server: srv, tg: tg, slack: slack, gh: gh, store: store}
}
