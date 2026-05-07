package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// When the user types `**bold**` in a rich-text comment input, the
// closing `**` MUST IMMEDIATELY mutate the in-memory ADF document —
// the inner run carries a `strong` mark, and the visible buffer has
// the marker characters stripped.
//
// The mutation MUST happen on the same Update tick as the closing `**`
// keystroke; we don't wait for submit. Convert-on-submit is the
// pattern this test prevents from silently re-emerging.
func TestRichInputAppliesBoldShortcutImmediately(t *testing.T) {
	ti := NewTextInput().Show("comment", "KAN-1", "Comment:", "")

	// Type "**bold**" one character at a time, mirroring real keystrokes.
	for _, r := range "**bold**" {
		m, _ := ti.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		ti = m.(TextInput)
	}

	// After the closing `**`, the visible buffer MUST be just "bold" —
	// the markers were stripped on the same tick as the close.
	if got := ti.input.Value(); got != "bold" {
		t.Fatalf("buffer not stripped of ** markers: %q", got)
	}

	// The editor's document MUST carry a `strong` mark on the inner run
	// — proof that the shortcut mutated the ADF, not just the display.
	if ti.rich == nil {
		t.Fatal("rich editor not initialized for comment action")
	}
	doc := ti.rich.Document()
	if len(doc.Content) == 0 {
		t.Fatalf("editor doc empty: %+v", doc)
	}
	first := doc.Content[0]
	if first.Type != "paragraph" {
		t.Fatalf("expected paragraph, got %q", first.Type)
	}
	// Find a text node with a `strong` mark and "bold" content.
	var found bool
	for _, c := range first.Content {
		if c.Type != "text" {
			continue
		}
		if c.Text != "bold" {
			continue
		}
		for _, m := range c.Marks {
			if m.Type == "strong" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("strong mark not applied to 'bold' run; doc=%+v", doc)
	}
}

// Typing plain text in a rich-text input still yields a valid ADF
// document — every character has been mirrored into the editor.
func TestRichInputMirrorsPlainKeystrokesIntoEditor(t *testing.T) {
	ti := NewTextInput().Show("comment", "KAN-1", "Comment:", "")
	for _, r := range "hello" {
		m, _ := ti.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		ti = m.(TextInput)
	}
	doc := ti.rich.Document()
	got := concatTextNodes(doc)
	if got != "hello" {
		t.Fatalf("editor text = %q, want hello", got)
	}
}

// Plain-text actions ("transition", "worklog") MUST NOT initialize the
// editor; they pass through the textinput.Model as-is.
func TestPlainActionDoesNotInitEditor(t *testing.T) {
	ti := NewTextInput().Show("transition", "KAN-1", "Transition:", "")
	if ti.rich != nil {
		t.Fatal("plain action should not initialize rich editor")
	}
}

// Backspace after a closed `**bold**` shortcut MUST preserve marks on
// the remaining text. The previous implementation rebuilt the editor
// on every non-append edit, which destroyed mark history.
func TestBackspaceAfterClosedShortcutPreservesMarks(t *testing.T) {
	ti := NewTextInput().Show("comment", "KAN-1", "Comment:", "")
	for _, r := range "**bold**" {
		m, _ := ti.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		ti = m.(TextInput)
	}
	// Backspace 'd' from "bold".
	m, _ := ti.Update(tea.KeyPressMsg{Code: rune(127), Text: ""}) // backspace
	ti = m.(TextInput)
	if got := ti.input.Value(); got != "bol" {
		t.Fatalf("buffer after backspace = %q, want %q", got, "bol")
	}
	// The remaining "bol" MUST still carry the strong mark — the
	// "immediately mutates" contract implies marks persist across edits.
	doc := ti.rich.Document()
	first := doc.Content[0]
	var foundMarked bool
	for _, c := range first.Content {
		if c.Type == "text" && c.Text == "bol" {
			for _, m := range c.Marks {
				if m.Type == "strong" {
					foundMarked = true
				}
			}
		}
	}
	if !foundMarked {
		t.Fatalf("strong mark lost after backspace; doc=%+v", doc)
	}
}

// Two `**…**` sequences in the same input MUST each get their own
// strong run. The previous rebuild approach collapsed them into a
// single strong span.
func TestTwoBoldSequencesPreserveDistinctMarks(t *testing.T) {
	ti := NewTextInput().Show("comment", "KAN-1", "Comment:", "")
	for _, r := range "**foo** **bar**" {
		m, _ := ti.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		ti = m.(TextInput)
	}
	if got := ti.input.Value(); got != "foo bar" {
		t.Fatalf("buffer = %q, want %q", got, "foo bar")
	}
	// Both "foo" and "bar" MUST carry strong; the space between them
	// MUST be plain.
	doc := ti.rich.Document()
	first := doc.Content[0]
	var fooStrong, barStrong, spacePlain bool
	for _, c := range first.Content {
		if c.Type != "text" {
			continue
		}
		hasStrong := false
		for _, m := range c.Marks {
			if m.Type == "strong" {
				hasStrong = true
			}
		}
		switch {
		case c.Text == "foo" && hasStrong:
			fooStrong = true
		case c.Text == "bar" && hasStrong:
			barStrong = true
		case c.Text == " " && !hasStrong:
			spacePlain = true
		}
	}
	if !fooStrong || !barStrong {
		t.Fatalf("expected both 'foo' and 'bar' strong; doc=%+v", doc)
	}
	if !spacePlain {
		t.Fatalf("expected space plain between bold runs; doc=%+v", doc)
	}
}

// Typing literal asterisks (`*` only, not closed `**…**`) MUST be
// preserved as text without applying any marks. Closing detection
// MUST require a matching open.
func TestUnpairedAsterisksRemainLiteralText(t *testing.T) {
	ti := NewTextInput().Show("comment", "KAN-1", "Comment:", "")
	for _, r := range "a*b" {
		m, _ := ti.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		ti = m.(TextInput)
	}
	if got := ti.input.Value(); got != "a*b" {
		t.Fatalf("unpaired asterisks should be literal: %q", got)
	}
	doc := ti.rich.Document()
	first := doc.Content[0]
	for _, c := range first.Content {
		if c.Type == "text" && len(c.Marks) > 0 {
			t.Fatalf("no marks should be applied to unpaired asterisks; got %+v", c)
		}
	}
}
