package adf

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// Empty text nodes are invalid ADF (a text node's text must be non-empty) and
// Jira rejects the whole document, so Normalize must strip them while leaving
// real text and other nodes untouched.
func TestNormalizeRemovesEmptyTextNodes(t *testing.T) {
	doc := Document{
		Type:    "doc",
		Version: 1,
		Content: []Node{
			{Type: "paragraph", Content: []Node{
				{Type: "text", Text: "before"},
				{Type: "text", Text: ""},
				{Type: "text", Text: "after"},
			}},
		},
	}

	got, warnings := Normalize(doc)

	para := got.Content[0]
	if len(para.Content) != 2 {
		t.Fatalf("paragraph kept %d inline nodes, want 2 (empty text dropped): %+v", len(para.Content), para.Content)
	}
	if para.Content[0].Text != "before" || para.Content[1].Text != "after" {
		t.Fatalf("surviving text nodes = %q/%q, want before/after", para.Content[0].Text, para.Content[1].Text)
	}
	if len(warnings) != 1 || warnings[0].Lossy {
		t.Fatalf("want exactly one non-lossy normalization warning, got %+v", warnings)
	}
}

// Whitespace is real content — only the truly-empty string is stripped.
func TestNormalizeKeepsWhitespaceText(t *testing.T) {
	doc := Document{Type: "doc", Version: 1, Content: []Node{
		{Type: "paragraph", Content: []Node{{Type: "text", Text: " "}}},
	}}
	got, warnings := Normalize(doc)
	if len(got.Content[0].Content) != 1 {
		t.Fatalf("whitespace-only text node was dropped; it is valid content")
	}
	if len(warnings) != 0 {
		t.Fatalf("no repair expected for whitespace text, got %+v", warnings)
	}
}

// The empty text node may be nested arbitrarily deep (the real-world trigger is
// a blank table cell: table → row → cell → paragraph → empty text). Removing it
// leaves an empty paragraph, which is valid ADF, so the cell stays well-formed.
func TestNormalizeRecursesAndLeavesValidEmptyParent(t *testing.T) {
	doc := Document{Type: "doc", Version: 1, Content: []Node{
		{Type: "table", Content: []Node{
			{Type: "tableRow", Content: []Node{
				{Type: "tableCell", Content: []Node{
					{Type: "paragraph", Content: []Node{{Type: "text", Text: ""}}},
				}},
			}},
		}},
	}}

	got, _ := Normalize(doc)

	cell := got.Content[0].Content[0].Content[0]
	if len(cell.Content) != 1 || cell.Content[0].Type != "paragraph" {
		t.Fatalf("table cell should still hold its (now-empty) paragraph: %+v", cell.Content)
	}
	if len(cell.Content[0].Content) != 0 {
		t.Fatalf("paragraph should be empty after the empty text node is dropped: %+v", cell.Content[0].Content)
	}

	// The normalized document must pass strict validation that the original
	// (with the empty text node) does not.
	if _, err := ValidateDoc(got, adfmode.ModeStrict); err != nil {
		t.Fatalf("normalized doc failed strict validation: %v", err)
	}
}

// Normalize must not mutate its input — callers may still need the original
// (e.g. dry-run preview vs. submitted form).
func TestNormalizeDoesNotMutateInput(t *testing.T) {
	doc := Document{Type: "doc", Version: 1, Content: []Node{
		{Type: "paragraph", Content: []Node{
			{Type: "text", Text: "x"},
			{Type: "text", Text: ""},
		}},
	}}
	Normalize(doc)
	if len(doc.Content[0].Content) != 2 {
		t.Fatalf("Normalize mutated its input: paragraph now has %d nodes", len(doc.Content[0].Content))
	}
}

// A doc with no empty text nodes is returned unchanged with no warnings.
func TestNormalizeNoOpWhenClean(t *testing.T) {
	doc := Document{Type: "doc", Version: 1, Content: []Node{
		{Type: "paragraph", Content: []Node{{Type: "text", Text: "clean"}}},
	}}
	_, warnings := Normalize(doc)
	if len(warnings) != 0 {
		t.Fatalf("clean doc produced normalization warnings: %+v", warnings)
	}
}
