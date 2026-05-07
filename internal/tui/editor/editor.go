// Package editor is the in-TUI ADF-native rich-text editor. The
// internal buffer is the typed adf.Document, not a Markdown string.
// Markdown-style key shortcuts mutate the document in place — there is
// no convert-on-submit step.
//
// Opaque blocks (unknown ADF nodes preserved on parse) are treated as
// protected: they render but cannot be edited or deleted.
package editor

import "github.com/matcra587/jira-cli/pkg/adf"

// Position describes a caret position inside the document.
type Position struct {
	Block  int // index into Document.Content
	Offset int // rune offset within the current block
}

// Options configure a new editor.
type Options struct {
	// Initial document; if zero, a fresh empty doc with one paragraph is
	// created.
	Initial adf.Document
}

// Editor is the public ADF editor model. The Bubble Tea model wraps it.
type Editor struct {
	doc    adf.Document
	caret  Position
	selAll bool // simplistic select-all flag for the strong/em test path
}

// New constructs an editor with an optional initial document.
func New(opts Options) *Editor {
	doc := opts.Initial
	if doc.Type == "" {
		doc = adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
			{Type: "paragraph"},
		}}
	}
	e := &Editor{doc: doc}
	// Caret defaults to end of last block.
	if len(doc.Content) > 0 {
		e.caret = Position{Block: len(doc.Content) - 1, Offset: blockTextLen(doc.Content[len(doc.Content)-1])}
	}
	return e
}

// Document returns a snapshot of the typed ADF doc.
func (e *Editor) Document() adf.Document { return e.doc }

// MoveTo updates the caret position. Bounds are not strictly enforced
// here; the renderer clamps when rendering.
func (e *Editor) MoveTo(p Position) {
	e.caret = p
	e.selAll = false
}

// SelectAll selects every text node in the current block. Future versions
// will model selection as a (Position, Position) range; for now this flag
// is enough to test the strong/em shortcut path.
func (e *Editor) SelectAll() { e.selAll = true }

// InsertText appends text to the current block. If the current block is
// not a text-bearing paragraph/heading, a new paragraph is appended.
func (e *Editor) InsertText(s string) {
	if s == "" {
		return
	}
	if len(e.doc.Content) == 0 {
		e.doc.Content = append(e.doc.Content, adf.Node{Type: "paragraph"})
		e.caret = Position{Block: 0, Offset: 0}
	}
	block := &e.doc.Content[e.caret.Block]
	if !canHoldText(block.Type) {
		// Append a fresh paragraph.
		e.doc.Content = append(e.doc.Content, adf.Node{Type: "paragraph"})
		e.caret.Block = len(e.doc.Content) - 1
		e.caret.Offset = 0
		block = &e.doc.Content[e.caret.Block]
	}
	if len(block.Content) == 0 {
		block.Content = append(block.Content, adf.Node{Type: "text", Text: s})
	} else {
		// Append to the last text node if possible; else add a new one.
		last := &block.Content[len(block.Content)-1]
		if last.Type == "text" && len(last.Marks) == 0 {
			last.Text += s
		} else {
			block.Content = append(block.Content, adf.Node{Type: "text", Text: s})
		}
	}
	e.caret.Offset = blockTextLen(*block)
}

// HandleShortcut applies a Markdown-style shortcut to the editor state.
// The supported shortcuts are the documented Markdown shortcuts plus
// the few inline marks the tests exercise; the full MVP set lives
// behind individual helper methods invoked from the Bubble Tea key
// handler.
func (e *Editor) HandleShortcut(token string) {
	if e.IsAtOpaqueBlock() {
		return // opaque blocks are protected
	}
	switch token {
	case "# ":
		e.replaceCurrentBlock(adf.Node{Type: "heading", Attrs: map[string]any{"level": 1}})
	case "## ":
		e.replaceCurrentBlock(adf.Node{Type: "heading", Attrs: map[string]any{"level": 2}})
	case "### ":
		e.replaceCurrentBlock(adf.Node{Type: "heading", Attrs: map[string]any{"level": 3}})
	case "**":
		e.toggleMark("strong")
	case "*", "_":
		e.toggleMark("em")
	case "`":
		e.toggleMark("code")
	case "~~":
		e.toggleMark("strike")
	}
}

// replaceCurrentBlock swaps the type/attrs of the caret's current block,
// preserving any text content already present.
func (e *Editor) replaceCurrentBlock(replacement adf.Node) {
	if e.caret.Block >= len(e.doc.Content) {
		return
	}
	cur := e.doc.Content[e.caret.Block]
	replacement.Content = cur.Content
	e.doc.Content[e.caret.Block] = replacement
}

// toggleMark applies a mark to the selected text. With the placeholder
// SelectAll flag we just attach the mark to every text node in the
// current block.
func (e *Editor) toggleMark(name string) {
	if e.caret.Block >= len(e.doc.Content) {
		return
	}
	block := &e.doc.Content[e.caret.Block]
	if !e.selAll {
		// Without a real selection range, attach to the last text node so
		// the user sees the toggle take effect on the just-typed run.
		if len(block.Content) > 0 {
			last := &block.Content[len(block.Content)-1]
			if last.Type == "text" {
				last.Marks = appendMarkUnique(last.Marks, name)
			}
		}
		return
	}
	for i := range block.Content {
		if block.Content[i].Type == "text" {
			block.Content[i].Marks = appendMarkUnique(block.Content[i].Marks, name)
		}
	}
	e.selAll = false
}

func appendMarkUnique(marks []adf.Mark, name string) []adf.Mark {
	for _, m := range marks {
		if m.Type == name {
			return marks
		}
	}
	return append(marks, adf.Mark{Type: name})
}

// IsAtOpaqueBlock reports whether the caret currently sits on a block
// the editor cannot edit (an unknown ADF node preserved by Parse).
func (e *Editor) IsAtOpaqueBlock() bool {
	if e.caret.Block >= len(e.doc.Content) {
		return false
	}
	return isOpaqueType(e.doc.Content[e.caret.Block].Type)
}

// DeleteCurrentBlock removes the caret's block, EXCEPT when it is an
// opaque (protected) block. Returns true if anything was deleted.
func (e *Editor) DeleteCurrentBlock() bool {
	if e.IsAtOpaqueBlock() {
		return false
	}
	if e.caret.Block >= len(e.doc.Content) {
		return false
	}
	e.doc.Content = append(e.doc.Content[:e.caret.Block], e.doc.Content[e.caret.Block+1:]...)
	if e.caret.Block >= len(e.doc.Content) {
		e.caret.Block = len(e.doc.Content) - 1
		if e.caret.Block < 0 {
			e.caret.Block = 0
		}
	}
	return true
}

// canHoldText reports which block types accept inserted text directly.
func canHoldText(t string) bool {
	switch t {
	case "paragraph", "heading", "blockquote", "codeBlock":
		return true
	default:
		return false
	}
}

// isOpaqueType returns true when the block type is outside the MVP
// authoring set. Opaque blocks survive editing untouched.
func isOpaqueType(t string) bool {
	switch t {
	case
		"doc", "paragraph", "text", "heading",
		"bulletList", "orderedList", "listItem",
		"codeBlock", "blockquote", "hardBreak", "rule",
		"mention", "emoji", "date", "status", "inlineCard",
		"panel", "table", "tableRow", "tableCell", "tableHeader":
		return false
	}
	return true
}

// blockTextLen returns the total rune length of every text node inside a
// block — used to position the caret at end-of-block.
func blockTextLen(n adf.Node) int {
	if n.Type == "text" {
		return len([]rune(n.Text))
	}
	total := 0
	for _, c := range n.Content {
		total += blockTextLen(c)
	}
	return total
}

// ApplyMarkToCurrentParagraphRange applies markType to the rune range
// [start, end) of the current paragraph's flat text content. Existing
// marks on adjacent runs are preserved — text nodes are split at the
// range boundaries so the affected run carries exactly the new mark
// (in addition to whatever marks it already had) and nothing else
// changes.
//
// Returns false when the caret is not on a text-bearing block, or
// when [start, end) is empty / out of bounds. Idempotent on a range
// already carrying the same mark.
//
// Used by the TUI's Markdown-style shortcut layer to apply
// strong/em/code/strike marks to a precise span without rebuilding
// the document and losing earlier marks.
func (e *Editor) ApplyMarkToCurrentParagraphRange(start, end int, markType string) bool {
	if start >= end {
		return false
	}
	if e.caret.Block >= len(e.doc.Content) {
		return false
	}
	block := &e.doc.Content[e.caret.Block]
	if !canHoldText(block.Type) {
		return false
	}
	// Walk the block's text-node sequence, splitting where the range
	// boundaries fall. Build a new content slice with the splits applied
	// so we don't mutate during iteration.
	newContent := make([]adf.Node, 0, len(block.Content)+2)
	cursor := 0
	for _, n := range block.Content {
		if n.Type != "text" {
			newContent = append(newContent, n)
			continue
		}
		runes := []rune(n.Text)
		nodeStart := cursor
		nodeEnd := cursor + len(runes)
		// Three regions per node: [0, leftCut) → unchanged
		//                         [leftCut, rightCut) → mark applied
		//                         [rightCut, end) → unchanged
		// All in node-local rune offsets.
		leftCut := clampInt(start-nodeStart, 0, len(runes))
		rightCut := clampInt(end-nodeStart, 0, len(runes))
		if leftCut == rightCut {
			// Range doesn't intersect this node.
			newContent = append(newContent, n)
			cursor = nodeEnd
			continue
		}
		if leftCut > 0 {
			newContent = append(newContent, adf.Node{
				Type:  "text",
				Text:  string(runes[:leftCut]),
				Marks: cloneMarks(n.Marks),
			})
		}
		marked := adf.Node{
			Type:  "text",
			Text:  string(runes[leftCut:rightCut]),
			Marks: cloneMarks(n.Marks),
		}
		marked.Marks = appendMarkUnique(marked.Marks, markType)
		newContent = append(newContent, marked)
		if rightCut < len(runes) {
			newContent = append(newContent, adf.Node{
				Type:  "text",
				Text:  string(runes[rightCut:]),
				Marks: cloneMarks(n.Marks),
			})
		}
		cursor = nodeEnd
	}
	block.Content = newContent
	return true
}

// DeleteLastRune removes the trailing rune from the current paragraph
// without disturbing marks on the remaining text. If the trailing run
// becomes empty, the empty text node is removed. Returns false when
// the paragraph is already empty.
//
// Used by the TUI form's keystroke mirror so backspace doesn't have
// to rebuild the editor (which would destroy mark history).
func (e *Editor) DeleteLastRune() bool {
	if e.caret.Block >= len(e.doc.Content) {
		return false
	}
	block := &e.doc.Content[e.caret.Block]
	if !canHoldText(block.Type) {
		return false
	}
	for i := len(block.Content) - 1; i >= 0; i-- {
		n := &block.Content[i]
		if n.Type != "text" {
			continue
		}
		runes := []rune(n.Text)
		if len(runes) == 0 {
			block.Content = append(block.Content[:i], block.Content[i+1:]...)
			continue
		}
		n.Text = string(runes[:len(runes)-1])
		if n.Text == "" {
			block.Content = append(block.Content[:i], block.Content[i+1:]...)
		}
		// Update caret offset best-effort.
		if e.caret.Offset > 0 {
			e.caret.Offset--
		}
		return true
	}
	return false
}

// CurrentParagraphLen returns the rune length of all text in the
// current paragraph — used by the form to map between visible buffer
// length and editor offsets when applying range marks.
func (e *Editor) CurrentParagraphLen() int {
	if e.caret.Block >= len(e.doc.Content) {
		return 0
	}
	block := e.doc.Content[e.caret.Block]
	return blockTextLen(block)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func cloneMarks(in []adf.Mark) []adf.Mark {
	if len(in) == 0 {
		return nil
	}
	out := make([]adf.Mark, len(in))
	copy(out, in)
	return out
}
