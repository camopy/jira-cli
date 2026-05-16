package cli

import (
	"strings"
	"testing"
)

func TestSanitizeTerminalTextStripsControlBytes(t *testing.T) {
	in := "hel\x00lo\x07world\x1b"
	got := SanitizeTerminalText(in)
	if strings.ContainsAny(got, "\x00\x07\x1b") {
		t.Fatalf("control bytes survived sanitization: %q", got)
	}
	if got != "helloworld" {
		t.Fatalf("SanitizeTerminalText = %q, want helloworld", got)
	}
}

func TestSanitizeTerminalTextStripsANSISequences(t *testing.T) {
	in := "\x1b[31mred\x1b[0m"
	got := SanitizeTerminalText(in)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ANSI escape survived: %q", got)
	}
	if got != "red" {
		t.Fatalf("SanitizeTerminalText = %q, want red", got)
	}
}

// A completion candidate is one tab-separated record per line. Embedded
// tabs and newlines in the candidate fields would corrupt that grammar,
// so they must be collapsed to spaces.
func TestSanitizeCompletionFieldRemovesTabAndNewline(t *testing.T) {
	got := SanitizeCompletionField("multi\nline\tvalue")
	if strings.ContainsAny(got, "\t\n\r") {
		t.Fatalf("tab/newline survived: %q", got)
	}
	if got != "multi line value" {
		t.Fatalf("SanitizeCompletionField = %q, want \"multi line value\"", got)
	}
}

// Hyperlink construction must sanitize the inner text so a Jira-supplied
// control byte cannot break the OSC 8 span open/close pair.
func TestHyperlinkSanitizesInnerText(t *testing.T) {
	got := Hyperlink("https://example.com", "te\x1b]8;;evilxt")
	if strings.Contains(got, "evil") && strings.Count(got, "\x1b]8;;") > 2 {
		t.Fatalf("injected OSC 8 span survived: %q", got)
	}
}
