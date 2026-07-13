package storage

import (
	"context"
	"time"
)

type Repository interface {
	RecordDelivery(ctx context.Context, deliveryID string) (isDuplicate bool, err error)
	DeleteDelivery(ctx context.Context, deliveryID string) error
	CleanOldDeliveries(ctx context.Context, olderThan time.Duration) error

	CreateRetry(ctx context.Context, r CICDRetry) (id int64, err error)
	GetRetry(ctx context.Context, id int64) (*CICDRetry, error)
	UpdateRetryStatus(ctx context.Context, id int64, status RetryStatus) error
	ExpirePendingRetries(ctx context.Context, olderThan time.Duration) ([]CICDRetry, error)

	UpsertPRSession(ctx context.Context, s PRSession) error
	GetPRSession(ctx context.Context, chatID, messageID int64) (*PRSession, error)
	GetActivePRSession(ctx context.Context, chatID int64) (*PRSession, error)
	ExpireInProgressSessions(ctx context.Context, olderThan time.Duration) ([]PRSession, error)
}
