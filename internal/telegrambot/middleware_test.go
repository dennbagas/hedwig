package telegrambot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestExtractUserID(t *testing.T) {
	tests := []struct {
		name   string
		update *models.Update
		wantID int64
		wantOK bool
	}{
		{
			name:   "message from user",
			update: &models.Update{Message: &models.Message{From: &models.User{ID: 111}}},
			wantID: 111,
			wantOK: true,
		},
		{
			name:   "callback query from user",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{From: models.User{ID: 222}}},
			wantID: 222,
			wantOK: true,
		},
		{
			name:   "message with nil From",
			update: &models.Update{Message: &models.Message{From: nil}},
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "callback query with zero-value From",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{From: models.User{ID: 0}}},
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "neither message nor callback query",
			update: &models.Update{},
			wantID: 0,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := ExtractUserID(tt.update)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("ExtractUserID() = (%d, %v), want (%d, %v)", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestIsAllowed(t *testing.T) {
	allowlist := []int64{111, 222, 333}

	tests := []struct {
		name   string
		update *models.Update
		want   bool
	}{
		{
			name:   "allowed user via message",
			update: &models.Update{Message: &models.Message{From: &models.User{ID: 222}}},
			want:   true,
		},
		{
			name:   "allowed user via callback query",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{From: models.User{ID: 333}}},
			want:   true,
		},
		{
			name:   "disallowed user",
			update: &models.Update{Message: &models.Message{From: &models.User{ID: 999}}},
			want:   false,
		},
		{
			name:   "no extractable user ID",
			update: &models.Update{},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowed(allowlist, tt.update); got != tt.want {
				t.Errorf("IsAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAllowedEmptyAllowlist(t *testing.T) {
	update := &models.Update{Message: &models.Message{From: &models.User{ID: 111}}}
	if IsAllowed(nil, update) {
		t.Error("IsAllowed() with empty allowlist = true, want false")
	}
}

func TestGenerateRequestID(t *testing.T) {
	id := GenerateRequestID()
	if len(id) != 16 { // 8 bytes hex-encoded = 16 chars
		t.Errorf("GenerateRequestID() length = %d, want 16", len(id))
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("GenerateRequestID() = %q, contains non-hex character %q", id, r)
			break
		}
	}
}

func TestGenerateRequestIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateRequestID()
		if seen[id] {
			t.Fatalf("GenerateRequestID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}
