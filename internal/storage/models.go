package storage

import "time"

type RetryStatus string

const (
	RetryStatusPending RetryStatus = "pending"
	RetryStatusRetried RetryStatus = "retried"
	RetryStatusExpired RetryStatus = "expired"
)

type CICDRetry struct {
	ID        int64
	ChatID    int64
	MessageID int64
	RunID     int64
	Repo      string
	Status    RetryStatus
	CreatedAt time.Time
}

type PRStep string

const (
	PRStepSelectRepo   PRStep = "select_repo"
	PRStepEnterTitle   PRStep = "enter_title"
	PRStepEnterMessage PRStep = "enter_message"
	PRStepConfirm      PRStep = "confirm"
	PRStepDone         PRStep = "done"
	PRStepCancelled    PRStep = "cancelled"
)

type PRStatus string

const (
	PRStatusInProgress PRStatus = "in_progress"
	PRStatusCompleted  PRStatus = "completed"
	PRStatusCancelled  PRStatus = "cancelled"
	PRStatusExpired    PRStatus = "expired"
)

type PRSession struct {
	ChatID    int64
	MessageID int64
	Step      PRStep
	Repo      string
	PRTitle   string
	PRMessage string
	Status    PRStatus
	UpdatedAt time.Time
}
