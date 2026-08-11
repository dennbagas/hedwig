package slackbot

import "testing"

func TestFormatQuoted(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "header and detail block",
			in:   "✅ Push to `main`\n\nRepository: acme/widgets\nAuthor: alice",
			want: "✅ Push to `main`\n\n> Repository: acme/widgets\n> Author: alice",
		},
		{
			name: "no blank line quotes everything",
			in:   "one line only",
			want: "> one line only",
		},
		{
			name: "multiple paragraphs after header — only first blank line splits",
			in:   "Header\n\nA: 1\n\nB: 2",
			want: "Header\n\n> A: 1\n> \n> B: 2",
		},
		{
			name: "empty string",
			in:   "",
			want: "> ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatQuoted(tt.in); got != tt.want {
				t.Errorf("FormatQuoted(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
