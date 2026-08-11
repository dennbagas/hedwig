package notify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewTemplateLoaderReadsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "push.tmpl"), []byte(`hello {{.}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte(`not a template`), 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := newTemplateLoader(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("newTemplateLoader() error = %v", err)
	}

	text, err := l.render("push", "world")
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if text != "hello world" {
		t.Errorf("render() = %q, want %q", text, "hello world")
	}

	// .txt file must not be loaded.
	text, err = l.render("ignored", nil)
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if text != "" {
		t.Errorf("render(ignored) = %q, want empty (non-.tmpl files must be skipped)", text)
	}
}

func TestNewTemplateLoaderInvalidDir(t *testing.T) {
	_, err := newTemplateLoader("/nonexistent/path", zerolog.Nop())
	if err == nil {
		t.Fatal("newTemplateLoader() error = nil, want error for nonexistent directory")
	}
}

func TestNewTemplateLoaderSyntaxError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.tmpl"), []byte(`{{.Unclosed`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newTemplateLoader(dir, zerolog.Nop())
	if err == nil {
		t.Fatal("newTemplateLoader() error = nil, want parse error")
	}
}

func TestNewTemplateLoaderFromStrings(t *testing.T) {
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"push": `repo={{.Repo}}`,
	})
	if err != nil {
		t.Fatalf("newTemplateLoaderFromStrings() error = %v", err)
	}

	text, err := l.render("push", struct{ Repo string }{"acme/widgets"})
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if text != "repo=acme/widgets" {
		t.Errorf("render() = %q, want %q", text, "repo=acme/widgets")
	}
}

func TestNewTemplateLoaderFromStringsSyntaxError(t *testing.T) {
	_, err := newTemplateLoaderFromStrings(map[string]string{
		"push": `{{.Unclosed`,
	})
	if err == nil {
		t.Fatal("newTemplateLoaderFromStrings() error = nil, want parse error")
	}
}

func TestRenderMissingTemplate(t *testing.T) {
	l, err := newTemplateLoaderFromStrings(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := l.render("push", nil)
	if err != nil {
		t.Fatalf("render() error = %v, want nil for missing template", err)
	}
	if text != "" {
		t.Errorf("render() = %q, want empty string for missing template", text)
	}
}

func TestRenderEmptyOutputTrimmed(t *testing.T) {
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"push": `  ` + "\n\t\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := l.render("push", nil)
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if text != "" {
		t.Errorf("render() = %q, want empty string after trimming whitespace-only output", text)
	}
}

func TestQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"one line", "> one line"},
		{"line1\nline2", "> line1\n> line2"},
		{"a\n\nb", "> a\n> \n> b"}, // blank lines get quoted too, matching Slack's own per-line requirement
		{"", "> "},
	}
	for _, tt := range tests {
		if got := quote(tt.in); got != tt.want {
			t.Errorf("quote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestQuoteUsableFromTemplate(t *testing.T) {
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"push": `{{quote (printf "Repository: %s\nAuthor: %s" .Repo .Pusher)}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := l.render("push", struct{ Repo, Pusher string }{"acme/widgets", "alice"})
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	want := "> Repository: acme/widgets\n> Author: alice"
	if text != want {
		t.Errorf("render() = %q, want %q", text, want)
	}
}

func TestRenderExecutionError(t *testing.T) {
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"push": `{{.Missing}}`, // accesses a field that doesn't exist on the given type
	})
	if err != nil {
		t.Fatal(err)
	}
	// Passing a concrete type with no "Missing" field triggers an execution error.
	_, err = l.render("push", struct{ Repo string }{"acme/widgets"})
	if err == nil {
		t.Fatal("render() error = nil, want execution error for undefined field")
	}
}
