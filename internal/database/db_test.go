package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	return count == 1
}

func TestOpenCreatesExpectedTables(t *testing.T) {
	db := newTestDB(t)

	for _, table := range []string{"webhook_deliveries", "cicd_retries", "cicd_retry_targets"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q was not created", table)
		}
	}
}

func TestMigrationDownAndUpPreservesTelegramTargets(t *testing.T) {
	db := newTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO cicd_retries (id, run_id, repo, status, workflow_name) VALUES (1, 55, 'acme/widgets', 'pending', 'Build')`,
	); err != nil {
		t.Fatalf("seed cicd_retries: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO cicd_retry_targets (retry_id, platform, chat_ref, message_ref, message_text) VALUES (1, 'telegram', '111', '222', 'msg')`,
	); err != nil {
		t.Fatalf("seed cicd_retry_targets: %v", err)
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("iofs.New() error = %v", err)
	}
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		t.Fatalf("sqlite.WithInstance() error = %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		t.Fatalf("migrate.NewWithInstance() error = %v", err)
	}

	if err := m.Steps(-1); err != nil {
		t.Fatalf("migrate down one step error = %v", err)
	}
	if tableExists(t, db, "cicd_retry_targets") {
		t.Error("cicd_retry_targets should be dropped after migrating down")
	}

	var chatID, messageID int64
	var messageText string
	if err := db.QueryRow(`SELECT chat_id, message_id, message_text FROM cicd_retries WHERE id = 1`).Scan(&chatID, &messageID, &messageText); err != nil {
		t.Fatalf("query downgraded cicd_retries: %v", err)
	}
	if chatID != 111 || messageID != 222 || messageText != "msg" {
		t.Errorf("downgraded row chat_id=%d message_id=%d message_text=%q, want 111/222/msg (backfilled from the telegram target)", chatID, messageID, messageText)
	}

	if err := m.Steps(1); err != nil {
		t.Fatalf("migrate back up error = %v", err)
	}
	if !tableExists(t, db, "cicd_retry_targets") {
		t.Error("cicd_retry_targets should exist again after migrating back up")
	}
	var chatRef, messageRef, retryMessageText string
	if err := db.QueryRow(`SELECT chat_ref, message_ref, message_text FROM cicd_retry_targets WHERE retry_id = 1 AND platform = 'telegram'`).Scan(&chatRef, &messageRef, &retryMessageText); err != nil {
		t.Fatalf("query re-upgraded cicd_retry_targets: %v", err)
	}
	if chatRef != "111" || messageRef != "222" || retryMessageText != "msg" {
		t.Errorf("re-upgraded target chat_ref=%q message_ref=%q message_text=%q, want 111/222/msg", chatRef, messageRef, retryMessageText)
	}
}

func TestOpenSetsWALMode(t *testing.T) {
	db := newTestDB(t)

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestOpenSetsBusyTimeout(t *testing.T) {
	db := newTestDB(t)

	var timeoutMS int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeoutMS); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeoutMS != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeoutMS)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first handle: %v", err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() on the same path error = %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	if !tableExists(t, db2, "cicd_retries") {
		t.Error("expected tables to still exist after reopening")
	}
}
