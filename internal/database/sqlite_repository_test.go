package database

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func newTestRepo(t *testing.T) (*sql.DB, Repository) {
	t.Helper()
	db := newTestDB(t)
	return db, NewSQLiteRepository(db)
}

func TestRecordDeliveryDedup(t *testing.T) {
	_, repo := newTestRepo(t)
	ctx := context.Background()

	dup, err := repo.RecordDelivery(ctx, "delivery-1")
	if err != nil {
		t.Fatalf("first RecordDelivery() error = %v", err)
	}
	if dup {
		t.Error("first RecordDelivery() isDuplicate = true, want false")
	}

	dup, err = repo.RecordDelivery(ctx, "delivery-1")
	if err != nil {
		t.Fatalf("second RecordDelivery() error = %v", err)
	}
	if !dup {
		t.Error("second RecordDelivery() with the same ID isDuplicate = false, want true")
	}

	// A different ID is unaffected.
	dup, err = repo.RecordDelivery(ctx, "delivery-2")
	if err != nil {
		t.Fatalf("RecordDelivery() for a different ID error = %v", err)
	}
	if dup {
		t.Error("RecordDelivery() for a distinct ID isDuplicate = true, want false")
	}
}

func TestDeleteDelivery(t *testing.T) {
	_, repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordDelivery(ctx, "delivery-1"); err != nil {
		t.Fatalf("RecordDelivery() error = %v", err)
	}
	if err := repo.DeleteDelivery(ctx, "delivery-1"); err != nil {
		t.Fatalf("DeleteDelivery() error = %v", err)
	}

	dup, err := repo.RecordDelivery(ctx, "delivery-1")
	if err != nil {
		t.Fatalf("RecordDelivery() after delete error = %v", err)
	}
	if dup {
		t.Error("RecordDelivery() after DeleteDelivery isDuplicate = true, want false (record should be gone)")
	}
}

func TestDeleteDeliveryNonexistentIsNotAnError(t *testing.T) {
	_, repo := newTestRepo(t)
	if err := repo.DeleteDelivery(context.Background(), "never-recorded"); err != nil {
		t.Errorf("DeleteDelivery() on a nonexistent ID error = %v, want nil", err)
	}
}

func TestCleanOldDeliveries(t *testing.T) {
	db, repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordDelivery(ctx, "old"); err != nil {
		t.Fatalf("RecordDelivery(old) error = %v", err)
	}
	if _, err := repo.RecordDelivery(ctx, "recent"); err != nil {
		t.Fatalf("RecordDelivery(recent) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	if _, err := db.ExecContext(ctx,
		`UPDATE webhook_deliveries SET received_at = ? WHERE delivery_id = ?`, backdated, "old"); err != nil {
		t.Fatalf("backdate old delivery: %v", err)
	}

	if err := repo.CleanOldDeliveries(ctx, 24*time.Hour); err != nil {
		t.Fatalf("CleanOldDeliveries() error = %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhook_deliveries WHERE delivery_id = ?`, "old").Scan(&count); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if count != 0 {
		t.Error("old delivery should have been cleaned up")
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhook_deliveries WHERE delivery_id = ?`, "recent").Scan(&count); err != nil {
		t.Fatalf("count recent: %v", err)
	}
	if count != 1 {
		t.Error("recent delivery should still be present")
	}
}

func TestCreateAndGetRetry(t *testing.T) {
	_, repo := newTestRepo(t)
	ctx := context.Background()

	id, err := repo.CreateRetry(ctx, CICDRetry{
		ChatID: 1, MessageID: 2, RunID: 99, Repo: "acme/widgets", Status: RetryStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if id == 0 {
		t.Fatal("CreateRetry() returned id = 0, want a positive id")
	}

	got, err := repo.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRetry() returned nil, want the created record")
	}
	if got.ChatID != 1 || got.MessageID != 2 || got.RunID != 99 || got.Repo != "acme/widgets" || got.Status != RetryStatusPending {
		t.Errorf("GetRetry() = %+v, want matching fields from CreateRetry", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetRetry().CreatedAt is zero, want it to be set")
	}
}

func TestGetRetryNotFound(t *testing.T) {
	_, repo := newTestRepo(t)
	got, err := repo.GetRetry(context.Background(), 999999)
	if err != nil {
		t.Fatalf("GetRetry() error = %v, want nil error for a not-found id", err)
	}
	if got != nil {
		t.Errorf("GetRetry() = %+v, want nil for a not-found id", got)
	}
}

func TestUpdateRetryStatus(t *testing.T) {
	_, repo := newTestRepo(t)
	ctx := context.Background()

	id, err := repo.CreateRetry(ctx, CICDRetry{ChatID: 1, MessageID: 2, RunID: 3, Repo: "a/b", Status: RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	if err := repo.UpdateRetryStatus(ctx, id, RetryStatusRetried); err != nil {
		t.Fatalf("UpdateRetryStatus() error = %v", err)
	}

	got, err := repo.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if got.Status != RetryStatusRetried {
		t.Errorf("Status = %q, want %q", got.Status, RetryStatusRetried)
	}
}

func TestExpirePendingRetries(t *testing.T) {
	db, repo := newTestRepo(t)
	ctx := context.Background()

	oldID, err := repo.CreateRetry(ctx, CICDRetry{ChatID: 1, MessageID: 1, RunID: 1, Repo: "a/b", Status: RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(old) error = %v", err)
	}
	recentID, err := repo.CreateRetry(ctx, CICDRetry{ChatID: 2, MessageID: 2, RunID: 2, Repo: "a/b", Status: RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(recent) error = %v", err)
	}
	retriedID, err := repo.CreateRetry(ctx, CICDRetry{ChatID: 3, MessageID: 3, RunID: 3, Repo: "a/b", Status: RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry(retried) error = %v", err)
	}
	if err := repo.UpdateRetryStatus(ctx, retriedID, RetryStatusRetried); err != nil {
		t.Fatalf("UpdateRetryStatus(retried) error = %v", err)
	}

	backdated := time.Now().Add(-48 * time.Hour)
	if _, err := db.ExecContext(ctx,
		`UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, oldID); err != nil {
		t.Fatalf("backdate old retry: %v", err)
	}
	// The already-retried row is also old, to prove status (not just age) gates expiry.
	if _, err := db.ExecContext(ctx,
		`UPDATE cicd_retries SET created_at = ? WHERE id = ?`, backdated, retriedID); err != nil {
		t.Fatalf("backdate retried row: %v", err)
	}

	expired, err := repo.ExpirePendingRetries(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("ExpirePendingRetries() error = %v", err)
	}
	if len(expired) != 1 || expired[0].ID != oldID {
		t.Errorf("ExpirePendingRetries() = %+v, want exactly the old pending row (id=%d)", expired, oldID)
	}

	oldRow, err := repo.GetRetry(ctx, oldID)
	if err != nil {
		t.Fatalf("GetRetry(old) error = %v", err)
	}
	if oldRow.Status != RetryStatusExpired {
		t.Errorf("old row status = %q, want %q", oldRow.Status, RetryStatusExpired)
	}

	recentRow, err := repo.GetRetry(ctx, recentID)
	if err != nil {
		t.Fatalf("GetRetry(recent) error = %v", err)
	}
	if recentRow.Status != RetryStatusPending {
		t.Errorf("recent row status = %q, want unchanged %q", recentRow.Status, RetryStatusPending)
	}

	retriedRow, err := repo.GetRetry(ctx, retriedID)
	if err != nil {
		t.Fatalf("GetRetry(retried) error = %v", err)
	}
	if retriedRow.Status != RetryStatusRetried {
		t.Errorf("already-retried row status = %q, want unchanged %q", retriedRow.Status, RetryStatusRetried)
	}
}

func TestExpirePendingRetriesNoneExpired(t *testing.T) {
	_, repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.CreateRetry(ctx, CICDRetry{ChatID: 1, MessageID: 1, RunID: 1, Repo: "a/b", Status: RetryStatusPending}); err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	expired, err := repo.ExpirePendingRetries(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("ExpirePendingRetries() error = %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("ExpirePendingRetries() = %+v, want none expired", expired)
	}
}
