package notify

import "testing"

func TestEscEscapesHTMLSpecialChars(t *testing.T) {
	cases := map[string]string{
		"plain text":            "plain text",
		"<b>bold</b>":           "&lt;b&gt;bold&lt;/b&gt;",
		"Tom & Jerry":           "Tom &amp; Jerry",
		`<a href="x">click</a>`: "&lt;a href=&#34;x&#34;&gt;click&lt;/a&gt;",
	}
	for in, want := range cases {
		if got := esc(in); got != want {
			t.Errorf("esc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHtmlLinkEscapesTextAndURL(t *testing.T) {
	got := htmlLink(`<script>alert(1)</script>`, `https://example.com/?a=1&b=2`)
	want := `<a href="https://example.com/?a=1&amp;b=2">&lt;script&gt;alert(1)&lt;/script&gt;</a>`
	if got != want {
		t.Errorf("htmlLink() = %q, want %q", got, want)
	}
}

func TestCapitalizeOnlyTouchesLowercaseASCII(t *testing.T) {
	cases := map[string]string{
		"tag":    "Tag",
		"branch": "Branch",
		"":       "",
		"Tag":    "Tag",
	}
	for in, want := range cases {
		if got := capitalize(in); got != want {
			t.Errorf("capitalize(%q) = %q, want %q", in, got, want)
		}
	}
}
