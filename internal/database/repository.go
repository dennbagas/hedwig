package database

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
}
