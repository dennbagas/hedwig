package httpserver

import (
	"path/filepath"
	"testing"

	"github.com/btse/hedwig/internal/githubapp/githubapptest"
	"github.com/btse/hedwig/internal/notify"
	"github.com/btse/hedwig/internal/retry"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot/telegrambottest"
	"go.uber.org/zap"
)

// testServer wires a real *Server to a real notify.Dispatcher and
// retry.Handler, backed by fake GitHub/Telegram clients and a real
// temp-file SQLite repository — only the true external network edges are
// faked, so these tests exercise the actual routing/business logic.
type testServer struct {
	*Server
	tg    *telegrambottest.FakeClient
	gh    *githubapptest.FakeClient
	store storage.Repository
}

func newTestServer(t *testing.T, allowedUserIDs []int64, telegramSecret string) *testServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	store := storage.NewSQLiteRepository(db)

	tg := telegrambottest.New()
	gh := githubapptest.New()

	retryH := retry.New(store, tg, gh, zap.NewNop())
	notifyD := notify.NewDispatcher(tg, 999, zap.NewNop())
	notify.RegisterAll(notifyD, tg, retryH, 999)

	srv := New(gh, store, notifyD, retryH, allowedUserIDs, telegramSecret, "/healthz", "/webhooks/telegram", zap.NewNop())

	return &testServer{Server: srv, tg: tg, gh: gh, store: store}
}
