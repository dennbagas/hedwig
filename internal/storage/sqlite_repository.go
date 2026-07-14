package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type sqliteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

func (r *sqliteRepository) RecordDelivery(ctx context.Context, deliveryID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO webhook_deliveries (delivery_id) VALUES (?)`, deliveryID)
	if err != nil {
		return false, fmt.Errorf("record delivery: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 0, nil
}

func (r *sqliteRepository) DeleteDelivery(ctx context.Context, deliveryID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM webhook_deliveries WHERE delivery_id = ?`, deliveryID)
	return err
}

func (r *sqliteRepository) CleanOldDeliveries(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM webhook_deliveries WHERE received_at < ?`, cutoff)
	return err
}

func (r *sqliteRepository) CreateRetry(ctx context.Context, retry CICDRetry) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO cicd_retries (chat_id, message_id, run_id, repo, status) VALUES (?, ?, ?, ?, ?)`,
		retry.ChatID, retry.MessageID, retry.RunID, retry.Repo, string(retry.Status))
	if err != nil {
		return 0, fmt.Errorf("create retry: %w", err)
	}
	return res.LastInsertId()
}

func (r *sqliteRepository) GetRetry(ctx context.Context, id int64) (*CICDRetry, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, chat_id, message_id, run_id, repo, status, created_at FROM cicd_retries WHERE id = ?`, id)
	return scanRetry(row)
}

func (r *sqliteRepository) UpdateRetryStatus(ctx context.Context, id int64, status RetryStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cicd_retries SET status = ? WHERE id = ?`, string(status), id)
	return err
}

func (r *sqliteRepository) ExpirePendingRetries(ctx context.Context, olderThan time.Duration) ([]CICDRetry, error) {
	cutoff := time.Now().Add(-olderThan)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT id, chat_id, message_id, run_id, repo, status, created_at
		 FROM cicd_retries WHERE status = 'pending' AND created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var retries []CICDRetry
	for rows.Next() {
		var retry CICDRetry
		var status string
		if err := rows.Scan(&retry.ID, &retry.ChatID, &retry.MessageID, &retry.RunID, &retry.Repo, &status, &retry.CreatedAt); err != nil {
			return nil, err
		}
		retry.Status = RetryStatus(status)
		retries = append(retries, retry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(retries) > 0 {
		ids := make([]interface{}, len(retries))
		placeholders := make([]byte, 0, len(retries)*2)
		for i, ret := range retries {
			ids[i] = ret.ID
			if i > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
		}
		query := `UPDATE cicd_retries SET status = 'expired' WHERE id IN (` + string(placeholders) + `)`
		if _, err := tx.ExecContext(ctx, query, ids...); err != nil {
			return nil, err
		}
	}

	return retries, tx.Commit()
}

func scanRetry(row *sql.Row) (*CICDRetry, error) {
	var r CICDRetry
	var status string
	err := row.Scan(&r.ID, &r.ChatID, &r.MessageID, &r.RunID, &r.Repo, &status, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan retry: %w", err)
	}
	r.Status = RetryStatus(status)
	return &r, nil
}
