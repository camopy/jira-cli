package unit

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/pkg/adf"
)

func TestMarkdownADFMarkdownRoundTrip(t *testing.T) {
	doc, err := adf.FromMarkdown("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdown() error = %v", err)
	}
	out := adf.ToMarkdown(doc)
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Fatalf("round trip markdown = %q", out)
	}
}
