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
	// ClaimPendingRetry atomically transitions id from pending to to, and
	// reports whether this caller won the race (false means someone else
	// already claimed or the row wasn't pending). Used to make concurrent
	// taps of the same retry button — possible now that one retry can have
	// a live button on multiple platforms at once — safe against a double
	// rerun.
	ClaimPendingRetry(ctx context.Context, id int64, to RetryStatus) (claimed bool, err error)

	CreateRetryTarget(ctx context.Context, t RetryTarget) error
	ListRetryTargets(ctx context.Context, retryID int64) ([]RetryTarget, error)
}
