package unit

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

func TestADFFormattedPlainAndMarkdownRendering(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "hello"}}}}}
	if got := adf.ToPlain(doc); got != "hello" {
		t.Fatalf("ToPlain() = %q", got)
	}
	if got := adf.ToMarkdown(doc); strings.TrimSpace(got) != "hello" {
		t.Fatalf("ToMarkdown() = %q", got)
	}
	if len(adf.ToFormatted(doc)) == 0 {
		t.Fatal("ToFormatted() returned no segments")
	}
}

func TestADFRenderingPreservesFormattedNodesListsCodeAndLinks(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "heading", Attrs: map[string]any{"level": 2}, Content: []adf.Node{{Type: "text", Text: "Title"}}},
		{Type: "paragraph", Content: []adf.Node{
			{Type: "text", Text: "bold", Marks: []adf.Mark{{Type: "strong"}}},
			{Type: "text", Text: " link", Marks: []adf.Mark{{Type: "link", Attrs: map[string]any{"href": "https://example.com"}}}},
		}},
		{Type: "bulletList", Content: []adf.Node{{Type: "listItem", Content: []adf.Node{{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "item"}}}}}}},
		{Type: "codeBlock", Content: []adf.Node{{Type: "text", Text: "fmt.Println(\"hi\")\n"}}},
	}}

	markdown := adf.ToMarkdown(doc)
	for _, want := range []string{"## Title", "**bold**", "[ link](https://example.com)", "- item", "```"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("ToMarkdown() missing %q:\n%s", want, markdown)
		}
	}

	formatted := adf.ToFormatted(doc)
	for _, want := range []string{"heading", "strong", "link", "listItem", "codeBlock"} {
		if !hasFormattedKind(formatted, want) {
			t.Fatalf("ToFormatted() missing kind %q: %+v", want, formatted)
		}
	}
}

func TestADFMarkdownRenderingAcceptsJSONNumberHeadingLevel(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "heading", Attrs: map[string]any{"level": float64(3)}, Content: []adf.Node{{Type: "text", Text: "JSON heading"}}},
	}}
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "### JSON heading") {
		t.Fatalf("ToMarkdown() = %q, want level-three heading", got)
	}
}

func TestMarkdownToADFSupportsListsLinksAndCodeBlocks(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("# Title\n\n- [item](https://example.com)\n\n```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("converted markdown is not valid ADF: %v\n%+v", err, doc)
	}
	if len(doc.Content) != 3 {
		t.Fatalf("converted markdown content = %+v", doc.Content)
	}
	if doc.Content[1].Type != "bulletList" {
		t.Fatalf("markdown list converted to %q, want bulletList", doc.Content[1].Type)
	}
	if !strings.Contains(adf.ToMarkdown(doc), "[item](https://example.com)") {
		t.Fatalf("link did not round-trip: %s", adf.ToMarkdown(doc))
	}
}

func hasFormattedKind(segments []adf.Segment, kind string) bool {
	for _, segment := range segments {
		if segment.Kind == kind {
			return true
		}
	}
	return false
}
