package retry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hedwig/internal/database"
)

func TestSweepExpiresOldPendingRetries(t *testing.T) {
	db, store := newTestRepo(t)
	h, tg, slack, _, _ := newTestHandlerWithStore(t, store)
	ctx := context.Background()

	oldID, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 1, Repo: "a/b", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(old) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: oldID, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "11", MessageText: "old msg"}); err != nil {
		t.Fatalf("CreateRetryTarget(old) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: oldID, Platform: database.PlatformSlack, ChatRef: testSlackChannel, MessageRef: "111.222", MessageText: "old slack msg"}); err != nil {
		t.Fatalf("CreateRetryTarget(old slack) error = %v", err)
	}

	recentID, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 2, Repo: "a/b", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(recent) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: recentID, Platform: database.PlatformTelegram, ChatRef: "2", MessageRef: "22", MessageText: "recent msg"}); err != nil {
		t.Fatalf("CreateRetryTarget(recent) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	if _, err := db.ExecContext(ctx, `UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, oldID); err != nil {
		t.Fatalf("backdate old retry: %v", err)
	}

	sweep(ctx, h, 24*time.Hour)

	if len(tg.Sent) != 1 || tg.Sent[0].ChatID != 1 || tg.Sent[0].MessageID != 11 {
		t.Fatalf("tg.Sent = %+v, want exactly the old telegram target's message edited (chatID=1, messageID=11)", tg.Sent)
	}
	if !strings.Contains(tg.Sent[0].Text, "expired") {
		t.Errorf("telegram text = %q, want it to mention expiry", tg.Sent[0].Text)
	}
	if tg.Sent[0].Params.Keyboard == nil || len(tg.Sent[0].Params.Keyboard) != 0 {
		t.Errorf("keyboard = %+v, want empty keyboard to remove the button", tg.Sent[0].Params.Keyboard)
	}

	if len(slack.Sent) != 1 || slack.Sent[0].Channel != testSlackChannel || slack.Sent[0].Ts != "111.222" {
		t.Fatalf("slack.Sent = %+v, want exactly the old slack target's message edited", slack.Sent)
	}

	oldRec, err := store.GetRetry(ctx, oldID)
	if err != nil {
		t.Fatalf("GetRetry(old) error = %v", err)
	}
	if oldRec.Status != database.RetryStatusExpired {
		t.Errorf("old status = %q, want %q", oldRec.Status, database.RetryStatusExpired)
	}

	recentRec, err := store.GetRetry(ctx, recentID)
	if err != nil {
		t.Fatalf("GetRetry(recent) error = %v", err)
	}
	if recentRec.Status != database.RetryStatusPending {
		t.Errorf("recent status = %q, want unchanged %q", recentRec.Status, database.RetryStatusPending)
	}
}

func TestSweepContinuesAfterEditMessageError(t *testing.T) {
	db, store := newTestRepo(t)
	h, tg, _, _, _ := newTestHandlerWithStore(t, store)
	tg.EditMessageErr = errors.New("telegram is down")
	ctx := context.Background()

	firstID, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 1, Repo: "a/b", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(first) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: firstID, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "11", MessageText: "msg"}); err != nil {
		t.Fatalf("CreateRetryTarget(first) error = %v", err)
	}
	secondID, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 2, Repo: "a/b", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(second) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: secondID, Platform: database.PlatformTelegram, ChatRef: "2", MessageRef: "22", MessageText: "msg"}); err != nil {
		t.Fatalf("CreateRetryTarget(second) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	for _, id := range []int64{firstID, secondID} {
		if _, err := db.ExecContext(ctx, `UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, id); err != nil {
			t.Fatalf("backdate retry %d: %v", id, err)
		}
	}

	sweep(ctx, h, 24*time.Hour)

	for _, id := range []int64{firstID, secondID} {
		rec, err := store.GetRetry(ctx, id)
		if err != nil {
			t.Fatalf("GetRetry(%d) error = %v", id, err)
		}
		if rec.Status != database.RetryStatusExpired {
			t.Errorf("retry %d status = %q, want %q despite the EditMessage error", id, rec.Status, database.RetryStatusExpired)
		}
	}
}

func TestSweepNoExpiredRows(t *testing.T) {
	_, store := newTestRepo(t)
	h, tg, _, _, _ := newTestHandlerWithStore(t, store)
	ctx := context.Background()

	if _, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 1, Repo: "a/b", Status: database.RetryStatusPending}); err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	sweep(ctx, h, 24*time.Hour)

	if len(tg.Sent) != 0 {
		t.Errorf("tg.Sent = %+v, want none when nothing has expired", tg.Sent)
	}
}
