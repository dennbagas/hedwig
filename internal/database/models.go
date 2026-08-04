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
	RunID        int64
	Repo         string
	WorkflowName string
	Status       RetryStatus
	CreatedAt    time.Time
}

// RetryTarget is one platform's copy of a CI/CD retry notification — where
// its message lives, so the retry button can be found and edited again
// later (to strip it after a retry or on expiry), and the original rendered
// text for that platform, so the "retry sent"/"no longer valid" edit can be
// built from it. A single CICDRetry has one RetryTarget per platform it was
// posted to; MessageText is per-target (not on CICDRetry) because Telegram
// HTML and Slack mrkdwn render the same event as different text.
type RetryTarget struct {
	RetryID     int64
	Platform    string // "telegram" | "slack"
	ChatRef     string // Telegram chat_id (as string) or Slack channel_id
	MessageRef  string // Telegram message_id (as string) or Slack ts
	MessageText string
}

const (
	PlatformTelegram = "telegram"
	PlatformSlack    = "slack"
)
