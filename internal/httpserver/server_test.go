package httpserver

import (
	"path/filepath"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/githubapp/githubapptest"
	"hedwig/internal/notify"
	"hedwig/internal/retry"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/rs/zerolog"
)

// testServer wires a real *Server to a real notify.Dispatcher and
// retry.Handler, backed by fake GitHub/Telegram clients and a real
// temp-file SQLite repository — only the true external network edges are
// faked, so these tests exercise the actual routing/business logic.
type testServer struct {
	*Server
	tg    *telegrambottest.FakeClient
	gh    *githubapptest.FakeClient
	store database.Repository
}

func newTestServer(t *testing.T, allowedUserIDs []int64, telegramSecret string) *testServer {
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
	gh := githubapptest.New()

	retryH := retry.New(store, tg, gh, zerolog.Nop())
	notifyD := notify.NewDispatcher(tg, 999, zerolog.Nop())
	notify.RegisterAll(notifyD, tg, retryH, 999)

	srv := New(gh, store, notifyD, retryH, allowedUserIDs, telegramSecret, "/healthz", "/webhooks/telegram", zerolog.Nop())

	return &testServer{Server: srv, tg: tg, gh: gh, store: store}
}
