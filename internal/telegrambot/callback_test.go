package telegrambot

import "testing"

func TestEncodeDecodeCallbackRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		feature string
		action  string
		payload string
	}{
		{"simple", "retry", "trigger", "42"},
		{"payload with colons", "pr", "repo", "owner/repo:branch:extra"},
		{"empty payload", "retry", "trigger", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodeCallback(tt.feature, tt.action, tt.payload)

			feature, action, payload, err := DecodeCallback(data)
			if err != nil {
				t.Fatalf("DecodeCallback(%q) error = %v", data, err)
			}
			if feature != tt.feature {
				t.Errorf("feature = %q, want %q", feature, tt.feature)
			}
			if action != tt.action {
				t.Errorf("action = %q, want %q", action, tt.action)
			}
			if payload != tt.payload {
				t.Errorf("payload = %q, want %q", payload, tt.payload)
			}
		})
	}
}

func TestDecodeCallbackInvalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"wrong prefix", "notHedwig:retry:trigger:42"},
		{"too few parts", "hedwig:retry:trigger"},
		{"empty string", ""},
		{"prefix only", "hedwig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := DecodeCallback(tt.data)
			if err == nil {
				t.Fatalf("DecodeCallback(%q) error = nil, want error", tt.data)
			}
		})
	}
}

func TestEncodeCallbackFormat(t *testing.T) {
	got := EncodeCallback("retry", "trigger", "42")
	want := "hedwig:retry:trigger:42"
	if got != want {
		t.Errorf("EncodeCallback() = %q, want %q", got, want)
	}
}
