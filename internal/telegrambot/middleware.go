package telegrambot

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/go-telegram/bot/models"
)

// IsAllowed reports whether the user who triggered update is on the allowlist.
func IsAllowed(allowedIDs []int64, update *models.Update) bool {
	uid, ok := ExtractUserID(update)
	if !ok {
		return false
	}
	for _, id := range allowedIDs {
		if id == uid {
			return true
		}
	}
	return false
}

// ExtractUserID returns the user ID from a Telegram update regardless of type.
func ExtractUserID(update *models.Update) (int64, bool) {
	if update.Message != nil && update.Message.From != nil {
		return update.Message.From.ID, true
	}
	if update.CallbackQuery != nil && update.CallbackQuery.From.ID != 0 {
		return update.CallbackQuery.From.ID, true
	}
	return 0, false
}

// GenerateRequestID returns a short random hex string usable as a request correlation ID.
func GenerateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
