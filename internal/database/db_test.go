package database

import (
	"database/sql"
	"path/filepath"
	"testing"
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

	for _, table := range []string{"webhook_deliveries", "cicd_retries"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q was not created", table)
		}
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
