package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot/telegrambottest"
	"go.uber.org/zap"
)

func TestSweepExpiresOldPendingRetries(t *testing.T) {
	db, store := newTestRepo(t)
	tg := telegrambottest.New()
	ctx := context.Background()

	oldID, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 11, RunID: 1, Repo: "a/b", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(old) error = %v", err)
	}
	recentID, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 2, MessageID: 22, RunID: 2, Repo: "a/b", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(recent) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	if _, err := db.ExecContext(ctx, `UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, oldID); err != nil {
		t.Fatalf("backdate old retry: %v", err)
	}

	sweep(ctx, store, tg, 24*time.Hour, zap.NewNop())

	if len(tg.RemovedKeyboards) != 1 || tg.RemovedKeyboards[0].ChatID != 1 || tg.RemovedKeyboards[0].MessageID != 11 {
		t.Errorf("RemovedKeyboards = %+v, want exactly the old row's (chatID=1, messageID=11)", tg.RemovedKeyboards)
	}

	oldRec, err := store.GetRetry(ctx, oldID)
	if err != nil {
		t.Fatalf("GetRetry(old) error = %v", err)
	}
	if oldRec.Status != storage.RetryStatusExpired {
		t.Errorf("old status = %q, want %q", oldRec.Status, storage.RetryStatusExpired)
	}

	recentRec, err := store.GetRetry(ctx, recentID)
	if err != nil {
		t.Fatalf("GetRetry(recent) error = %v", err)
	}
	if recentRec.Status != storage.RetryStatusPending {
		t.Errorf("recent status = %q, want unchanged %q", recentRec.Status, storage.RetryStatusPending)
	}
}

func TestSweepContinuesAfterRemoveKeyboardError(t *testing.T) {
	db, store := newTestRepo(t)
	tg := telegrambottest.New()
	tg.RemoveKeyboardErr = errors.New("telegram is down")
	ctx := context.Background()

	firstID, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 11, RunID: 1, Repo: "a/b", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(first) error = %v", err)
	}
	secondID, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 2, MessageID: 22, RunID: 2, Repo: "a/b", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(second) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	for _, id := range []int64{firstID, secondID} {
		if _, err := db.ExecContext(ctx, `UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, id); err != nil {
			t.Fatalf("backdate retry %d: %v", id, err)
		}
	}

	// Should not panic and should still process both rows in storage even
	// though RemoveKeyboard fails for each.
	sweep(ctx, store, tg, 24*time.Hour, zap.NewNop())

	for _, id := range []int64{firstID, secondID} {
		rec, err := store.GetRetry(ctx, id)
		if err != nil {
			t.Fatalf("GetRetry(%d) error = %v", id, err)
		}
		if rec.Status != storage.RetryStatusExpired {
			t.Errorf("retry %d status = %q, want %q despite the RemoveKeyboard error", id, rec.Status, storage.RetryStatusExpired)
		}
	}
}

func TestSweepNoExpiredRows(t *testing.T) {
	_, store := newTestRepo(t)
	tg := telegrambottest.New()
	ctx := context.Background()

	if _, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 1, RunID: 1, Repo: "a/b", Status: storage.RetryStatusPending}); err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	sweep(ctx, store, tg, 24*time.Hour, zap.NewNop())

	if len(tg.RemovedKeyboards) != 0 {
		t.Errorf("RemovedKeyboards = %+v, want none when nothing has expired", tg.RemovedKeyboards)
	}
}
