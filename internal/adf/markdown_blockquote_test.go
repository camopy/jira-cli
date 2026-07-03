package adf_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// A blockquote converts to the ADF blockquote node wrapping its blocks.
func TestFromMarkdownBlockquoteShape(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("> first line\n>\n> second para\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	quote := doc.Content[0]
	if quote.Type != "blockquote" {
		t.Fatalf("node type = %q, want blockquote", quote.Type)
	}
	if len(quote.Content) != 2 {
		t.Fatalf("blockquote has %d children, want 2 paragraphs", len(quote.Content))
	}
	for _, child := range quote.Content {
		if child.Type != "paragraph" {
			t.Errorf("blockquote child type = %q, want paragraph", child.Type)
		}
	}
}

// The original failure case: "- >50 keys" parses as a blockquote nested in
// a list item. ADF list items cannot contain blockquotes, so the quoted
// content is hoisted into the item with a non-lossy downgrade — the create
// must succeed in strict mode, not abort.
func TestFromMarkdownBlockquoteInListItemHoisted(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- >50 keys\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want exactly the hoist downgrade", warnings)
	}
	if warnings[0].Lossy {
		t.Error("hoist downgrade must be Lossy=false: the content survives")
	}
	item := doc.Content[0].Content[0]
	if item.Type != "listItem" {
		t.Fatalf("node = %+v, want listItem", item)
	}
	for _, child := range item.Content {
		if child.Type == "blockquote" {
			t.Fatalf("listItem must not contain a blockquote; got %+v", item.Content)
		}
	}
	text := item.Content[0].Content[0].Text
	if !strings.Contains(text, "50 keys") {
		t.Fatalf("hoisted text = %q, want the quoted content preserved", text)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("strict validation rejected hoisted doc: %v", err)
	}
}

// Nested quotes flatten into the parent (the schema forbids blockquote in
// blockquote) and quoted headings become paragraphs, both non-lossy.
func TestFromMarkdownBlockquoteDegradesForbiddenChildren(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("> outer\n> > inner\n\n> # quoted heading\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	for _, w := range warnings {
		if w.Lossy {
			t.Errorf("downgrade must be non-lossy; got %+v", w)
		}
	}
	for _, quote := range doc.Content {
		for _, child := range quote.Content {
			if child.Type == "blockquote" || child.Type == "heading" {
				t.Fatalf("blockquote child %q is schema-invalid", child.Type)
			}
		}
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("strict validation rejected degraded doc: %v", err)
	}
}

// Bold-wrapped inline code is an invalid ADF mark pair: the converter keeps
// code, drops the decorative mark, and reports a lossy source-mapped
// warning (strict aborts on it; best-effort proceeds with the sanitized
// marks and a valid document).
func TestFromMarkdownCodeMarkConflictSanitized(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- **`Tree()`** walks\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want exactly the mark-conflict warning", warnings)
	}
	w := warnings[0]
	if !w.Lossy {
		t.Error("mark-conflict warning must be Lossy=true so strict aborts")
	}
	if !strings.Contains(w.Message, "strong") {
		t.Errorf("message = %q, want the dropped mark named", w.Message)
	}
	if !strings.Contains(w.Message, "line 1") || !strings.Contains(w.Message, "Tree()") {
		t.Errorf("message = %q, want source line and snippet", w.Message)
	}
	if w.Path == "" {
		t.Error("warning path must carry the source position")
	}

	code := doc.Content[0].Content[0].Content[0].Content[0]
	if len(code.Marks) != 1 || code.Marks[0].Type != "code" {
		t.Fatalf("marks = %+v, want code only", code.Marks)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("sanitized doc must pass strict validation: %v", err)
	}
}

// A link-wrapped code span is the one legal combination and stays intact.
func TestFromMarkdownCodeInsideLinkKeepsBothMarks(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("[`docs`](https://example.com)\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("code+link is valid; unexpected warnings %+v", warnings)
	}
	node := doc.Content[0].Content[0]
	types := make(map[string]bool, len(node.Marks))
	for _, m := range node.Marks {
		types[m.Type] = true
	}
	if !types["code"] || !types["link"] {
		t.Fatalf("marks = %+v, want code + link", node.Marks)
	}
}

// Dropped constructs carry the source position and offending snippet so an
// author can find the input line without decoding a JSON path.
func TestFromMarkdownDroppedConstructIsSourceMapped(t *testing.T) {
	_, warnings, err := adf.FromMarkdownLossy("fine paragraph\n\n<div>raw</div>\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want one raw-HTML drop", warnings)
	}
	w := warnings[0]
	if !strings.Contains(w.Message, "line 3") || !strings.Contains(w.Message, "<div>raw</div>") {
		t.Errorf("message = %q, want line number and snippet", w.Message)
	}
	if !strings.Contains(w.Path, "line 3") {
		t.Errorf("path = %q, want source position", w.Path)
	}
}
