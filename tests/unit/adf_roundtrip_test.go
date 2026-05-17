package unit

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

func TestMarkdownADFMarkdownRoundTrip(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	out := adf.ToMarkdown(doc)
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Fatalf("round trip markdown = %q", out)
	}
}
