package notify

import (
	"fmt"
	"strings"
)

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	return ref
}

func htmlLink(text, url string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, url, text)
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
