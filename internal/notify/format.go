package notify

import (
	"fmt"
	"html"
	"strings"
)

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	return ref
}

// esc escapes text so it is safe to embed in a Telegram ParseMode=HTML message.
func esc(s string) string {
	return html.EscapeString(s)
}

// htmlLink builds an HTML anchor tag, escaping both the URL and the link text.
func htmlLink(text, url string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, esc(url), esc(text))
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

func reviewStateLabel(state string) string {
	switch strings.ToLower(state) {
	case "approved":
		return "Approved"
	case "changes_requested":
		return "Changes requested"
	case "commented":
		return "Commented"
	default:
		return state
	}
}
