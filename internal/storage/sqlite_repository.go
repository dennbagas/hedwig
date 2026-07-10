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

func (r *sqliteRepository) UpsertPRSession(ctx context.Context, s PRSession) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO pr_sessions (chat_id, message_id, step, repo, pr_title, pr_message, status, trigger_user, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		s.ChatID, s.MessageID, string(s.Step), s.Repo, s.PRTitle, s.PRMessage, string(s.Status), s.TriggerUser)
	return err
}

func (r *sqliteRepository) GetPRSession(ctx context.Context, chatID, messageID int64) (*PRSession, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT chat_id, message_id, step, repo, pr_title, pr_message, status, trigger_user, updated_at
		 FROM pr_sessions WHERE chat_id = ? AND message_id = ?`, chatID, messageID)
	return scanSession(row)
}

func (r *sqliteRepository) GetActivePRSession(ctx context.Context, chatID int64) (*PRSession, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT chat_id, message_id, step, repo, pr_title, pr_message, status, trigger_user, updated_at
		 FROM pr_sessions WHERE chat_id = ? AND status = 'in_progress'
		 ORDER BY updated_at DESC LIMIT 1`, chatID)
	return scanSession(row)
}

func (r *sqliteRepository) ExpireInProgressSessions(ctx context.Context, olderThan time.Duration) ([]PRSession, error) {
	cutoff := time.Now().Add(-olderThan)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT chat_id, message_id, step, repo, pr_title, pr_message, status, updated_at
		 FROM pr_sessions WHERE status = 'in_progress' AND updated_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []PRSession
	for rows.Next() {
		s, err := scanSessionFromRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(sessions) > 0 {
		for _, s := range sessions {
			if _, err := tx.ExecContext(ctx,
				`UPDATE pr_sessions SET status = 'expired' WHERE chat_id = ? AND message_id = ?`,
				s.ChatID, s.MessageID); err != nil {
				return nil, err
			}
		}
	}

	return sessions, tx.Commit()
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

func scanSession(row *sql.Row) (*PRSession, error) {
	var s PRSession
	var step, status string
	err := row.Scan(&s.ChatID, &s.MessageID, &step, &s.Repo, &s.PRTitle, &s.PRMessage, &status, &s.TriggerUser, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	s.Step = PRStep(step)
	s.Status = PRStatus(status)
	return &s, nil
}

func scanSessionFromRows(rows *sql.Rows) (*PRSession, error) {
	var s PRSession
	var step, status string
	if err := rows.Scan(&s.ChatID, &s.MessageID, &step, &s.Repo, &s.PRTitle, &s.PRMessage, &status, &s.TriggerUser, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Step = PRStep(step)
	s.Status = PRStatus(status)
	return &s, nil
}
