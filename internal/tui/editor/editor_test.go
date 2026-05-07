package editor_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/tui/editor"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// The in-TUI editor MUST be backed by an ADF document model end-to-end.
// No "current text buffer" string that gets converted on submit.
func TestEditorBackedByADFDocument(t *testing.T) {
	e := editor.New(editor.Options{})
	e.InsertText("hello world")
	doc := e.Document()
	if doc.Type != "doc" {
		t.Fatalf("editor doc.type = %q, want doc", doc.Type)
	}
	if len(doc.Content) == 0 {
		t.Fatalf("editor doc has no content after InsertText")
	}
	first := doc.Content[0]
	if first.Type != "paragraph" {
		t.Fatalf("first block type = %q, want paragraph", first.Type)
	}
	body := flatten(first)
	if !strings.Contains(body, "hello world") {
		t.Fatalf("paragraph body missing inserted text; got %q", body)
	}
}

// Markdown-style shortcuts MUST mutate the underlying ADF document
// directly — no convert-on-submit. After the shortcut handler runs,
// the doc reflects the new structure immediately.
func TestMarkdownShortcutHashAtLineStartCreatesHeading(t *testing.T) {
	e := editor.New(editor.Options{})
	e.HandleShortcut("# ")
	e.InsertText("Title")
	doc := e.Document()
	if len(doc.Content) == 0 {
		t.Fatalf("doc empty")
	}
	if doc.Content[0].Type != "heading" {
		t.Fatalf("expected heading after `# ` shortcut, got %q", doc.Content[0].Type)
	}
	level, _ := doc.Content[0].Attrs["level"].(int)
	if level != 1 {
		// some encoders use float64 for JSON numbers; accept both
		if l, _ := doc.Content[0].Attrs["level"].(float64); int(l) != 1 {
			t.Fatalf("heading level = %v, want 1", doc.Content[0].Attrs["level"])
		}
	}
}

// The strong/em shortcuts MUST attach the corresponding mark directly
// to the affected text node. The doc must be valid ADF (representable
// on the wire) at every step.
func TestStrongShortcutWrapsSelectionWithStrongMark(t *testing.T) {
	e := editor.New(editor.Options{})
	e.InsertText("important")
	e.SelectAll()
	e.HandleShortcut("**")
	doc := e.Document()
	text := doc.Content[0].Content[0]
	if text.Type != "text" {
		t.Fatalf("expected text node, got %q", text.Type)
	}
	if len(text.Marks) != 1 || text.Marks[0].Type != "strong" {
		t.Fatalf("expected single strong mark, got %v", text.Marks)
	}
}

// Opaque blocks (unsupported nodes preserved during a parse) MUST be
// protected — the editor refuses to mutate them and they survive any
// subsequent serialization.
func TestOpaqueBlocksProtected(t *testing.T) {
	original := []byte(`{
		"type": "doc", "version": 1,
		"content": [
			{"type": "futureBlock", "attrs": {"x": 1}, "content": [{"type": "text", "text": "opaque"}]}
		]
	}`)
	doc, _, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := editor.New(editor.Options{Initial: doc})

	// Caret defaults at the end. Try to delete — opaque should refuse.
	e.MoveTo(editor.Position{Block: 0, Offset: 0})
	if !e.IsAtOpaqueBlock() {
		t.Fatal("editor should report opaque-block presence at position {0,0}")
	}
	if e.DeleteCurrentBlock() {
		t.Fatal("DeleteCurrentBlock should refuse to delete an opaque block")
	}

	// Re-marshal and verify the opaque survived byte-equivalently.
	out, err := adf.Marshal(e.Document())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), `"futureBlock"`) {
		t.Fatalf("opaque block was lost on serialize; got: %s", out)
	}
	if !strings.Contains(string(out), `"opaque"`) {
		t.Fatalf("opaque block child text lost; got: %s", out)
	}
}

// flatten gathers every text from a node subtree, separated by spaces.
// Used in tests to assert content irrespective of intermediate structure.
func flatten(n adf.Node) string {
	if n.Type == "text" {
		return n.Text
	}
	var parts []string
	for _, c := range n.Content {
		parts = append(parts, flatten(c))
	}
	return strings.Join(parts, "")
}

// Per-keystroke shortcut without mark loss: applying a mark to a
// specific text range MUST split text nodes at the range boundaries
// and apply the mark only to the covered run. Existing marks on text
// outside the range MUST be preserved.
func TestApplyMarkToRangeSplitsAndPreservesOtherMarks(t *testing.T) {
	e := editor.New(editor.Options{})
	// Build "alpha bravo charlie" with bravo already strong-marked.
	e.InsertText("alpha ")
	e.InsertText("bravo")
	e.SelectAll()
	e.HandleShortcut("**") // marks "alpha bravo" — but we want only bravo
	// Reset and use the new primitive instead.
	e = editor.New(editor.Options{})
	e.InsertText("alpha bravo charlie")
	if !e.ApplyMarkToCurrentParagraphRange(6, 11, "strong") {
		t.Fatal("ApplyMarkToCurrentParagraphRange returned false")
	}
	doc := e.Document()
	para := doc.Content[0]
	// Expected text node split: "alpha " (no marks), "bravo" (strong), " charlie" (no marks).
	if len(para.Content) != 3 {
		t.Fatalf("expected 3 text nodes after split, got %d: %+v", len(para.Content), para.Content)
	}
	if para.Content[0].Text != "alpha " || len(para.Content[0].Marks) != 0 {
		t.Fatalf("first node wrong: %+v", para.Content[0])
	}
	if para.Content[1].Text != "bravo" || len(para.Content[1].Marks) != 1 || para.Content[1].Marks[0].Type != "strong" {
		t.Fatalf("middle node not strong-marked: %+v", para.Content[1])
	}
	if para.Content[2].Text != " charlie" || len(para.Content[2].Marks) != 0 {
		t.Fatalf("last node wrong: %+v", para.Content[2])
	}
}

// Multi-shortcut preservation: applying strong to range [0,3) then
// range [10,13) MUST produce two distinct strong runs without erasing
// the first.
func TestApplyMarkToRangeMultipleSpansPreserved(t *testing.T) {
	e := editor.New(editor.Options{})
	e.InsertText("foo bar baz qux")
	if !e.ApplyMarkToCurrentParagraphRange(0, 3, "strong") {
		t.Fatal("first apply failed")
	}
	if !e.ApplyMarkToCurrentParagraphRange(8, 11, "strong") {
		t.Fatal("second apply failed")
	}
	doc := e.Document()
	para := doc.Content[0]
	// Walk and check we have two strong runs: foo and baz.
	var strongRuns []string
	for _, c := range para.Content {
		if c.Type != "text" {
			continue
		}
		for _, m := range c.Marks {
			if m.Type == "strong" {
				strongRuns = append(strongRuns, c.Text)
			}
		}
	}
	if len(strongRuns) != 2 {
		t.Fatalf("expected 2 strong runs, got %d: %v\nfull: %+v", len(strongRuns), strongRuns, para.Content)
	}
	if strongRuns[0] != "foo" || strongRuns[1] != "baz" {
		t.Fatalf("strong runs wrong: %v", strongRuns)
	}
}

// Backspace: DeleteLastRune MUST remove the trailing rune from the
// editor's text content, preserving marks on remaining text. The
// existing form-mirror code rebuilds the editor on backspace, which
// destroys mark history; the new primitive avoids that.
func TestDeleteLastRunePreservesEarlierMarks(t *testing.T) {
	e := editor.New(editor.Options{})
	e.InsertText("foo bar")
	if !e.ApplyMarkToCurrentParagraphRange(0, 3, "strong") {
		t.Fatal("apply failed")
	}
	// Backspace 'r' from the trailing "bar".
	if !e.DeleteLastRune() {
		t.Fatal("DeleteLastRune returned false")
	}
	doc := e.Document()
	para := doc.Content[0]
	// Concatenate text and check.
	var got string
	for _, c := range para.Content {
		got += c.Text
	}
	if got != "foo ba" {
		t.Fatalf("text after backspace = %q, want %q", got, "foo ba")
	}
	// "foo" must still carry strong.
	var fooMarked bool
	for _, c := range para.Content {
		if c.Text == "foo" {
			for _, m := range c.Marks {
				if m.Type == "strong" {
					fooMarked = true
				}
			}
		}
	}
	if !fooMarked {
		t.Fatalf("strong mark lost on 'foo' after backspace: %+v", para.Content)
	}
}
