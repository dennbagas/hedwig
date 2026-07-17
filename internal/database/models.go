package database

import "time"

type RetryStatus string

const (
	RetryStatusPending RetryStatus = "pending"
	RetryStatusRetried RetryStatus = "retried"
	RetryStatusExpired RetryStatus = "expired"
)

type CICDRetry struct {
	ID           int64
	ChatID       int64
	MessageID    int64
	RunID        int64
	Repo         string
	WorkflowName string
	Status       RetryStatus
	CreatedAt    time.Time
}
