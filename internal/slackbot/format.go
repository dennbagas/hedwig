package slackbot

import "strings"

// FormatQuoted blockquotes the "detail" portion of a rendered notification
// for Slack's mrkdwn, without requiring any change to the template that
// produced the text. Every Hedwig notification template renders as a short
// header line, a blank line, then a block of detail lines (e.g.
// "Repository: ...\nAuthor: ...") — this splits on that first blank line
// and blockquotes everything after it, leaving the header unquoted.
//
// mrkdwn requires the "> " marker on every line (unlike GitHub-flavored
// markdown, which allows lazy multi-line continuation), so quoting a
// multi-line block means prefixing each line individually; that's done
// here once, in code, rather than by hand in every template.
//
// If there is no blank line in text, the whole string is quoted.
func FormatQuoted(text string) string {
	header, rest, found := strings.Cut(text, "\n\n")
	if !found {
		return quoteLines(text)
	}
	return header + "\n\n" + quoteLines(rest)
}

func quoteLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}
