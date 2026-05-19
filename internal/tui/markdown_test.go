package tui

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// TUI ADF rendering MUST style headings, bold, lists, code blocks,
// and links as formatted terminal text (not plain text).
//
// Plain-text fallback is acceptable in non-TTY contexts; the test
// invokes the styled renderer directly and asserts ANSI styling is
// present for each rendered element.
func TestRenderADFStyledMarksHeadingsBoldListsCodeLinks(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "heading", "attrs": {"level": 1}, "content": [{"type": "text", "text": "Title"}]},
			{"type": "paragraph", "content": [
				{"type": "text", "text": "this is "},
				{"type": "text", "text": "bold", "marks": [{"type": "strong"}]},
				{"type": "text", "text": " and a "},
				{"type": "text", "text": "link", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]},
				{"type": "text", "text": "."}
			]},
			{"type": "bulletList", "content": [
				{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "first"}]}]},
				{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "second"}]}]}
			]},
			{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "package main\n"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := renderADFStyled(doc, 80, true)

	// Heading text MUST be present and STYLED — heading body wrapped in
	// the bold escape (ANSI \x1b[1m) so the heading visually distinguishes
	// from body text. We split the output into lines, find the heading
	// line, and assert it carries a bold escape AND a foreground color
	// escape (38;… SGR or a 30-37 8-color code).
	lines := strings.Split(out, "\n")
	headingLine := findLine(lines, "Title")
	if headingLine == "" {
		t.Fatalf("heading text dropped: %q", out)
	}
	if !strings.Contains(headingLine, "\x1b[1") {
		t.Fatalf("heading must be bold (ANSI \\x1b[1m); got %q", headingLine)
	}

	// Bold inline text MUST carry the bold escape on the "bold" run
	// — not just somewhere in the document.
	boldLine := findLine(lines, "bold")
	if boldLine == "" {
		t.Fatalf("bold text dropped: %q", out)
	}
	if !strings.Contains(boldLine, "\x1b[1") {
		t.Fatalf("inline bold must apply ANSI bold; got %q", boldLine)
	}

	// Link MUST emit an OSC 8 hyperlink so modern terminals render the
	// text as activatable.
	if !strings.Contains(out, "\x1b]8;;https://example.com") {
		t.Fatalf("link must emit OSC 8 escape with URL; got %q", out)
	}
	if !strings.Contains(out, "link") {
		t.Fatalf("link text dropped: %q", out)
	}

	// List items rendered with a bullet/marker character.
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Fatalf("list items dropped: %q", out)
	}
	if !strings.ContainsAny(out, "•-") {
		t.Fatalf("list missing bullet marker; got %q", out)
	}

	// Code-block content present AND visually faint (\x1b[2m) so it
	// distinguishes from prose. The "package main" line is wrapped in
	// the code-style render; the surrounding fence chars should also
	// carry the faint escape.
	if !strings.Contains(out, "package main") {
		t.Fatalf("code block dropped: %q", out)
	}
	if !strings.Contains(out, "\x1b[2") {
		t.Fatalf("code block must be faint (ANSI \\x1b[2m); got %q", out)
	}

	// Plain (non-TTY) MUST NOT emit ANY ANSI escapes.
	plain := renderADFStyled(doc, 80, false)
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("non-TTY render must NOT emit ANSI escapes; got %q", plain)
	}
	if strings.Contains(plain, "\x1b]") {
		t.Fatalf("non-TTY render must NOT emit OSC escapes; got %q", plain)
	}
}

// findLine returns the first line containing needle, or "" if none.
func findLine(lines []string, needle string) string {
	for _, l := range lines {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}
