package notify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
)

// templateLoader holds parsed notification templates keyed by event type string.
type templateLoader struct {
	templates map[string]*template.Template
	logger    zerolog.Logger
}

// templateFuncs is shared by every template regardless of destination
// platform — a function unused by a given template (e.g. quote in a
// Telegram HTML template) is simply never called.
var templateFuncs = template.FuncMap{
	"quote": quote,
}

// quote prefixes every line of s with "> ", Slack mrkdwn's blockquote
// marker. mrkdwn requires the marker on each line (no lazy multi-line
// continuation like GitHub-flavored markdown), so this exists to keep
// templates from having to repeat ">" by hand on every field line.
func quote(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// newTemplateLoader reads all *.tmpl files from dir, parses them, and returns a templateLoader.
// Returns an error if any file fails to parse (syntax validation at startup).
// Event types with no corresponding file are silently absent — they are
// logged and skipped at render time.
func newTemplateLoader(dir string, logger zerolog.Logger) (*templateLoader, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir %q: %w", dir, err)
	}
	l := &templateLoader{
		templates: make(map[string]*template.Template),
		logger:    logger,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		eventType := strings.TrimSuffix(entry.Name(), ".tmpl")
		path := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read template %q: %w", path, err)
		}
		tmpl, err := template.New(eventType).Funcs(templateFuncs).Parse(string(src))
		if err != nil {
			return nil, fmt.Errorf("parse template %q: %w", path, err)
		}
		l.templates[eventType] = tmpl
	}
	return l, nil
}

// newTemplateLoaderFromStrings builds a templateLoader from an in-memory map (for tests).
func newTemplateLoaderFromStrings(m map[string]string) (*templateLoader, error) {
	l := &templateLoader{
		templates: make(map[string]*template.Template),
		logger:    zerolog.Nop(),
	}
	for eventType, src := range m {
		tmpl, err := template.New(eventType).Funcs(templateFuncs).Parse(src)
		if err != nil {
			return nil, fmt.Errorf("parse template %q: %w", eventType, err)
		}
		l.templates[eventType] = tmpl
	}
	return l, nil
}

// render executes the template for eventType with data and returns the trimmed
// output. Returns ("", nil) when no template is registered (logged at warn)
// or when the template output is empty/whitespace-only. Returns ("", err) on
// template execution failure.
func (l *templateLoader) render(eventType string, data any) (string, error) {
	tmpl, ok := l.templates[eventType]
	if !ok {
		l.logger.Warn().Str("event_type", eventType).Msg("no template configured, skipping notification")
		return "", nil
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", eventType, err)
	}
	return strings.TrimSpace(buf.String()), nil
}
