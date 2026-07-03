package adf_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// ADF forbids a table inside a list item or blockquote, but the Markdown
// shape is routine — a status bullet followed by its indented table. The
// converter hoists the cleanly converted table to the nearest valid
// position (after the enclosing block) with a non-lossy downgrade, so the
// default strict mutation mode accepts the document.

func TestFromMarkdownTableInListItemHoistsAfterList(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- status:\n\n  | a | b |\n  |---|---|\n  | 1 | 2 |\n\ntail")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	kinds := make([]string, 0, len(doc.Content))
	for _, n := range doc.Content {
		kinds = append(kinds, n.Type)
	}
	if strings.Join(kinds, ",") != "bulletList,table,paragraph" {
		t.Fatalf("table must hoist to directly after its list, got %v", kinds)
	}
	assertSingleHoistDowngrade(t, warnings, "table inside a list item")
	normalized, _ := adf.Normalize(doc)
	if _, verr := adf.ValidateDoc(normalized, adfmode.ModeStrict); verr != nil {
		t.Fatalf("hoisted document failed strict validation: %v", verr)
	}
}

func TestFromMarkdownTableInBlockquoteHoistsAfterQuote(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("> quoted intro\n>\n> | a |\n> |---|\n> | 1 |\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	kinds := make([]string, 0, len(doc.Content))
	for _, n := range doc.Content {
		kinds = append(kinds, n.Type)
	}
	if strings.Join(kinds, ",") != "blockquote,table" {
		t.Fatalf("table must hoist to directly after its blockquote, got %v", kinds)
	}
	assertSingleHoistDowngrade(t, warnings, "table inside a blockquote")
	normalized, _ := adf.Normalize(doc)
	if _, verr := adf.ValidateDoc(normalized, adfmode.ModeStrict); verr != nil {
		t.Fatalf("hoisted document failed strict validation: %v", verr)
	}
}

func TestFromMarkdownMultipleNestedTablesKeepDocumentOrder(t *testing.T) {
	input := "- one:\n\n  | a |\n  |---|\n  | 1 |\n\n- two:\n\n  | b |\n  |---|\n  | 2 |\n"
	doc, _, err := adf.FromMarkdownLossy(input)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	var tables int
	for _, n := range doc.Content {
		if n.Type == "table" {
			tables++
		}
		if n.Type == "bulletList" {
			for _, item := range n.Content {
				for _, child := range item.Content {
					if child.Type == "table" {
						t.Fatalf("no table may remain inside a list item: %+v", doc.Content)
					}
				}
			}
		}
	}
	if tables != 2 {
		t.Fatalf("both nested tables must survive the hoist, got %d in %+v", tables, doc.Content)
	}
}

func assertSingleHoistDowngrade(t *testing.T, warnings []adf.Warning, construct string) {
	t.Helper()
	var found *adf.Warning
	for i, w := range warnings {
		if w.NodeType == construct {
			found = &warnings[i]
		}
	}
	if found == nil {
		t.Fatalf("expected a %q downgrade warning, got %+v", construct, warnings)
	}
	if found.Lossy {
		t.Fatalf("the hoist preserves the table in full; warning must be non-lossy: %+v", *found)
	}
}
