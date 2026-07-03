package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// A GFM table must convert to the canonical ADF table shape (the same
// shape as the table golden): table → tableRow → tableHeader/tableCell →
// paragraph → text, with the standard table attrs.
func TestFromMarkdownTableShape(t *testing.T) {
	md := "| Key | Value |\n|---|---|\n| alpha | 1 |\n| beta | 2 |\n"
	doc, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if len(doc.Content) != 1 {
		t.Fatalf("doc.Content has %d nodes, want 1", len(doc.Content))
	}
	table := doc.Content[0]
	if table.Type != "table" {
		t.Fatalf("node type = %q, want table", table.Type)
	}
	if v, _ := table.Attrs["isNumberColumnEnabled"].(bool); v {
		t.Errorf("attrs.isNumberColumnEnabled = %v, want false", table.Attrs["isNumberColumnEnabled"])
	}
	if v, _ := table.Attrs["layout"].(string); v != "default" {
		t.Errorf("attrs.layout = %q, want default", v)
	}
	if len(table.Content) != 3 {
		t.Fatalf("table has %d rows, want 3 (header + 2 body)", len(table.Content))
	}

	header := table.Content[0]
	if header.Type != "tableRow" {
		t.Fatalf("first child type = %q, want tableRow", header.Type)
	}
	for _, cell := range header.Content {
		if cell.Type != "tableHeader" {
			t.Errorf("header row cell type = %q, want tableHeader", cell.Type)
		}
	}
	body := table.Content[1]
	if len(body.Content) != 2 {
		t.Fatalf("body row has %d cells, want 2", len(body.Content))
	}
	cell := body.Content[0]
	if cell.Type != "tableCell" {
		t.Fatalf("body cell type = %q, want tableCell", cell.Type)
	}
	if len(cell.Content) != 1 || cell.Content[0].Type != "paragraph" {
		t.Fatalf("cell content = %+v, want a single paragraph", cell.Content)
	}
	if got := cell.Content[0].Content[0].Text; got != "alpha" {
		t.Fatalf("cell text = %q, want alpha", got)
	}
}

// Inline marks survive inside cells, and an empty cell still satisfies the
// schema's one-block-child minimum via an empty paragraph.
func TestFromMarkdownTableCellInlinesAndEmptyCells(t *testing.T) {
	md := "| a | b |\n|---|---|\n| **bold** | |\n"
	doc, _, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	body := doc.Content[0].Content[1]
	bold := body.Content[0].Content[0].Content[0]
	if len(bold.Marks) != 1 || bold.Marks[0].Type != "strong" {
		t.Errorf("cell text marks = %+v, want strong", bold.Marks)
	}
	empty := body.Content[1]
	if len(empty.Content) != 1 || empty.Content[0].Type != "paragraph" {
		t.Fatalf("empty cell content = %+v, want one (empty) paragraph", empty.Content)
	}
	if len(empty.Content[0].Content) != 0 {
		t.Errorf("empty cell paragraph content = %+v, want none", empty.Content[0].Content)
	}
}

// Converted tables must pass strict ADF validation — header rows, body
// rows, empty cells and all.
func TestFromMarkdownTablePassesStrictValidation(t *testing.T) {
	for name, md := range map[string]string{
		"basic":      "| Key | Value |\n|---|---|\n| alpha | 1 |\n",
		"empty-cell": "| a | b |\n|---|---|\n| x | |\n",
		"marks":      "| a |\n|---|\n| `code` and [link](https://example.com) |\n",
	} {
		t.Run(name, func(t *testing.T) {
			doc, _, err := adf.FromMarkdownLossy(md)
			if err != nil {
				t.Fatalf("FromMarkdownLossy: %v", err)
			}
			if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
				t.Fatalf("strict validation rejected converted table: %v", err)
			}
		})
	}
}
