// Goldens MUST be original — derived from the pinned ADF schema and
// the per-node/per-mark Atlassian docs. Deriving from third-party
// converters is forbidden.
package adf_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// All MVP nodes & marks have a corresponding `<name>.json` fixture in
// the goldens/ subdir. The test loops over the union and asserts each
// fixture round-trips through Parse/Marshal byte-equivalently.
var goldenNames = []string{
	// Nodes
	"doc", "paragraph", "text", "heading",
	"bulletList", "orderedList", "listItem",
	"codeBlock", "blockquote", "hardBreak", "rule",
	"mention", "emoji", "date", "status", "inlineCard",
	"panel", "table",
	// Marks (each fixture is a paragraph carrying the mark)
	"mark_strong", "mark_em", "mark_strike", "mark_code", "mark_link",
	"mark_textColor", "mark_backgroundColor", "mark_subsup", "mark_underline",
}

// Every golden MUST round-trip through Parse → Marshal without
// changing the JSON shape (modulo map ordering, which we normalise).
func TestGoldenFixturesRoundTrip(t *testing.T) {
	for _, name := range goldenNames {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", "goldens", name+".json")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			doc, warnings, err := adf.Parse(original)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("MVP fixture should not emit warnings; got %d", len(warnings))
			}
			got, err := adf.Marshal(doc)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			requireSemanticJSONEqual(t, original, got)
		})
	}
}

// The `mention` golden MUST carry attrs.id (accountId) and attrs.text
// per the official mention page. Asserts the typed model preserves both.
func TestMentionGoldenCarriesAccountIDAndText(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("testdata", "goldens", "mention.json"))
	if err != nil {
		t.Fatalf("read mention.json: %v", err)
	}
	doc, _, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mention := findFirstNode(doc, "mention")
	if mention == nil {
		t.Fatal("mention node not found in fixture")
	}
	id, _ := mention.Attrs["id"].(string)
	text, _ := mention.Attrs["text"].(string)
	if id == "" {
		t.Fatal("mention.attrs.id missing — accountId is required")
	}
	if !strings.HasPrefix(text, "@") {
		t.Fatalf("mention.attrs.text should be the rendered display text starting with @; got %q", text)
	}
}

// Helpers

func requireSemanticJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got: %v\nbytes: %s", err, got)
	}
	if !reflect.DeepEqual(w, g) {
		wb, _ := json.MarshalIndent(w, "", "  ")
		gb, _ := json.MarshalIndent(g, "", "  ")
		t.Fatalf("not equal\nwant:\n%s\ngot:\n%s", wb, gb)
	}
}

func findFirstNode(doc adf.Document, nodeType string) *adf.Node {
	for i := range doc.Content {
		if n := findFirst(&doc.Content[i], nodeType); n != nil {
			return n
		}
	}
	return nil
}

func findFirst(n *adf.Node, t string) *adf.Node {
	if n.Type == t {
		return n
	}
	for i := range n.Content {
		if found := findFirst(&n.Content[i], t); found != nil {
			return found
		}
	}
	return nil
}
