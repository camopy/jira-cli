package unit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// TestADFToMarkdownLossyEmptyDocument verifies that an empty document
// produces an empty (non-nil-or-nil but length 0) lossy-construct list.
func TestADFToMarkdownLossyEmptyDocument(t *testing.T) {
	res := adf.ToMarkdownLossy(adf.Document{Type: "doc", Version: 1})
	if len(res.LossyConstructs) != 0 {
		t.Fatalf("LossyConstructs = %v, want empty", res.LossyConstructs)
	}
}

// TestADFToMarkdownLossyAllSupported verifies that a doc made entirely of
// renderer-supported nodes/marks produces no lossy entries.
func TestADFToMarkdownLossyAllSupported(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "heading", Attrs: map[string]any{"level": 2}, Content: []adf.Node{
			{Type: "text", Text: "Title"},
		}},
		{Type: "paragraph", Content: []adf.Node{
			{Type: "text", Text: "bold", Marks: []adf.Mark{{Type: "strong"}}},
			{Type: "text", Text: " plain"},
			{Type: "text", Text: " link", Marks: []adf.Mark{{Type: "link", Attrs: map[string]any{"href": "https://example.com"}}}},
		}},
		{Type: "bulletList", Content: []adf.Node{
			{Type: "listItem", Content: []adf.Node{
				{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "item"}}},
			}},
		}},
		{Type: "codeBlock", Content: []adf.Node{{Type: "text", Text: "println(\"x\")\n"}}},
	}}
	res := adf.ToMarkdownLossy(doc)
	if len(res.LossyConstructs) != 0 {
		t.Fatalf("LossyConstructs = %v, want empty for fully-supported doc", res.LossyConstructs)
	}
	if !strings.Contains(res.Markdown, "## Title") {
		t.Fatalf("Markdown missing heading: %q", res.Markdown)
	}
}

// TestADFToMarkdownLossyInlineCard verifies a doc that contains an
// inlineCard node surfaces it in LossyConstructs.
func TestADFToMarkdownLossyInlineCard(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{
			{Type: "text", Text: "see "},
			{Type: "inlineCard", Attrs: map[string]any{"url": "https://example.com/x"}},
		}},
	}}
	res := adf.ToMarkdownLossy(doc)
	if !reflect.DeepEqual(res.LossyConstructs, []string{"inlineCard"}) {
		t.Fatalf("LossyConstructs = %v, want [\"inlineCard\"]", res.LossyConstructs)
	}
}

// TestADFToMarkdownLossyMultipleConstructsSortedUnique verifies multiple
// distinct lossy constructs are sorted and de-duplicated.
func TestADFToMarkdownLossyMultipleConstructsSortedUnique(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		// custom panel — Atlassian-extended panel variant the renderer doesn't
		// distinguish from a plain block; we use the node type itself as the
		// marker. Use a non-MVP node name to ensure it's flagged.
		{Type: "expand", Attrs: map[string]any{"title": "details"}, Content: []adf.Node{
			{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "hidden"}}},
		}},
		{Type: "paragraph", Content: []adf.Node{
			{Type: "inlineCard", Attrs: map[string]any{"url": "https://example.com/y"}},
			{Type: "text", Text: " and "},
			{Type: "inlineCard", Attrs: map[string]any{"url": "https://example.com/z"}}, // duplicate dedupes
		}},
	}}
	res := adf.ToMarkdownLossy(doc)
	want := []string{"expand", "inlineCard"}
	if !reflect.DeepEqual(res.LossyConstructs, want) {
		t.Fatalf("LossyConstructs = %v, want %v (sorted unique)", res.LossyConstructs, want)
	}
}

// TestADFToMarkdownPreservesWrapperBehavior verifies the existing
// adf.ToMarkdown wrapper still returns just the rendered string and
// is byte-identical to ToMarkdownLossy(...).Markdown.
func TestADFToMarkdownPreservesWrapperBehavior(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "hello"}}},
	}}
	got := adf.ToMarkdown(doc)
	res := adf.ToMarkdownLossy(doc)
	if got != res.Markdown {
		t.Fatalf("ToMarkdown(doc) = %q, ToMarkdownLossy(doc).Markdown = %q (must match)", got, res.Markdown)
	}
}
