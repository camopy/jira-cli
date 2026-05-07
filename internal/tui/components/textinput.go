package components

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/editor"
	"github.com/matcra587/jira-cli/internal/tui/theme"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// InputSubmitted is emitted when the user presses Enter in a TextInput overlay.
// Action labels the originating intent (e.g. "comment", "worklog") so the
// parent App can dispatch the correct mutation.
//
// Rich-text actions (currently "comment") attach Document so the App
// can route the typed ADF document directly to Jira with no
// convert-on-submit step. Non-rich-text actions ("transition",
// "worklog") leave Document nil and the App reads Value as before.
type InputSubmitted struct {
	Action   string
	Value    string
	IssueKey string
	Document *adf.Document
}

// InputCancelled is emitted when the user dismisses a TextInput overlay.
type InputCancelled struct{}

// TextInput is a modal single-line input overlay.
//
// When Action is a rich-text action ("comment"), an editor.Editor
// instance is the source of truth. Plain keystrokes are mirrored as
// Editor.InsertText / Editor.DeleteLastRune as they arrive. Markdown-
// style shortcut tokens (`**…**`) are detected at their CLOSING `**` by
// scanning the visible buffer; the markers are stripped from BOTH the
// visible buffer AND the editor's text, and a `strong` mark is applied
// to the inner run via Editor.ApplyMarkToCurrentParagraphRange.
//
// Crucially, ApplyMarkToCurrentParagraphRange splits text nodes at the
// range boundaries — earlier marks (e.g., from a previous closed `**…**`
// run earlier in the same input) are PRESERVED. Backspace also routes
// through DeleteLastRune which preserves marks on the remaining text
// rather than rebuilding the editor from scratch.
//
// On submit, Editor.Document() is attached to InputSubmitted; there is
// no adf.FromMarkdown call at submit: the TUI submit path MUST send
// the current ADF document directly to Jira with no convert-on-submit
// step.
type TextInput struct {
	Visible  bool
	action   string
	issueKey string
	prompt   string
	input    textinput.Model

	// rich is the ADF source of truth when action is rich-text. nil for
	// plain-text actions ("transition", "worklog" duration, etc.).
	rich *editor.Editor
	// lastValue is the buffer state after the previous Update tick.
	// Used to detect what changed (append, backspace, or other).
	lastValue string
}

func NewTextInput() TextInput {
	ti := textinput.New()
	ti.CharLimit = 1024
	return TextInput{input: ti}
}

// Show makes the overlay visible with the given context. Reassign:
//
//	t = t.Show("comment", "PROJ-1", "Add comment:", "")
func (t TextInput) Show(action, issueKey, prompt, placeholder string) TextInput {
	t.Visible = true
	t.action = action
	t.issueKey = issueKey
	t.prompt = prompt
	t.input.Placeholder = placeholder
	t.input.SetValue("")
	t.input.Focus()
	t.lastValue = ""
	if isRichTextAction(action) {
		e := editor.New(editor.Options{})
		t.rich = e
	} else {
		t.rich = nil
	}
	return t
}

func (t TextInput) Init() tea.Cmd { return textinput.Blink }

func (t TextInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !t.Visible {
		return t, nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			id := t.issueKey
			action := t.action
			t.Visible = false
			t.input.Blur()
			var (
				value = t.input.Value()
				doc   *adf.Document
			)
			if t.rich != nil {
				t.syncEditorToBuffer()
				d := t.rich.Document()
				doc = &d
			}
			return t, func() tea.Msg {
				return InputSubmitted{Action: action, Value: value, IssueKey: id, Document: doc}
			}
		case "esc":
			t.Visible = false
			t.input.Blur()
			return t, func() tea.Msg { return InputCancelled{} }
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	if t.rich != nil {
		t.processRichKeystroke()
	}
	t.lastValue = t.input.Value()
	return t, cmd
}

// processRichKeystroke routes the most recent buffer change into the
// editor source of truth and fires Markdown-style shortcut
// transformations when their closing token arrives.
//
// Three buffer-change patterns are handled surgically (without
// rebuilding the editor and losing prior marks):
//
//  1. Append (cur is lastValue + suffix): InsertText the suffix.
//  2. Backspace at tail (cur is a strict prefix of lastValue):
//     DeleteLastRune for each removed char.
//  3. Other divergence (cursor jump, paste at non-tail): full rebuild
//     as a last resort. Mark history is lost in this case; documented
//     in agent_guide.md as a known limitation of the single-line
//     rich-text input.
func (t *TextInput) processRichKeystroke() {
	cur := t.input.Value()
	switch {
	case cur == t.lastValue:
		// no-op (e.g., a non-printable key the textinput swallowed).
	case len(cur) > len(t.lastValue) && strings.HasPrefix(cur, t.lastValue):
		t.rich.InsertText(cur[len(t.lastValue):])
	case len(cur) < len(t.lastValue) && strings.HasPrefix(t.lastValue, cur):
		// Backspace at tail — possibly multiple chars (e.g., word delete).
		drop := len([]rune(t.lastValue)) - len([]rune(cur))
		for i := 0; i < drop; i++ {
			t.rich.DeleteLastRune()
		}
	default:
		// Non-tail divergence. Full rebuild as documented fallback.
		t.rich = editor.New(editor.Options{})
		t.rich.InsertText(cur)
	}
	t.applyClosedBoldShortcut()
}

// syncEditorToBuffer ensures the editor's text content matches the
// visible buffer at submit time. Defensive backstop for any tail diff
// that processRichKeystroke didn't catch (e.g., programmatic SetValue).
func (t *TextInput) syncEditorToBuffer() {
	if t.rich == nil {
		return
	}
	cur := t.input.Value()
	have := concatTextNodes(t.rich.Document())
	switch {
	case cur == have:
		return
	case strings.HasPrefix(cur, have):
		t.rich.InsertText(cur[len(have):])
	case strings.HasPrefix(have, cur):
		drop := len([]rune(have)) - len([]rune(cur))
		for i := 0; i < drop; i++ {
			t.rich.DeleteLastRune()
		}
	default:
		t.rich = editor.New(editor.Options{})
		t.rich.InsertText(cur)
	}
}

// applyClosedBoldShortcut detects a freshly-closed `**…**` run at the
// trailing edge of the buffer and:
//  1. strips the surrounding `**` markers from the visible textinput
//  2. removes them from the editor via DeleteLastRune (×2 for the close)
//     and DeleteLastRune-then-reinsert for the opening
//  3. applies a `strong` mark to the precise inner range via
//     ApplyMarkToCurrentParagraphRange
//
// Earlier marks on the paragraph are preserved — the fix that closes
// the multi-shortcut and backspace bugs found in the second critique.
func (t *TextInput) applyClosedBoldShortcut() {
	cur := t.input.Value()
	if !strings.HasSuffix(cur, "**") {
		return
	}
	prefix := strings.TrimSuffix(cur, "**")
	openIdx := strings.LastIndex(prefix, "**")
	if openIdx < 0 || openIdx == len(prefix)-2 {
		return // no opening, or empty inner ("****")
	}
	inner := prefix[openIdx+2:]
	if inner == "" {
		return
	}

	// Strip both markers from the visible buffer.
	stripped := prefix[:openIdx] + inner
	t.input.SetValue(stripped)
	t.input.SetCursor(len(stripped))
	t.lastValue = stripped

	// Mirror the strip into the editor: the editor currently has the
	// pre-strip content (including the four `**` characters). Delete
	// 4 trailing runes from the right, then we still need to delete
	// the OPENING `**` markers which are NOT at the tail. The simplest
	// model that's still surgical is:
	//   1. Compute the editor's current rune length (= len(prefix) + 2).
	//   2. Delete the 2 trailing `**` (close) → editor text matches "prefix".
	//   3. Splice out the 2-rune opening `**` at openIdx by deleting and
	//      re-inserting the tail (which preserves marks on text outside
	//      that 2-rune window).
	//   4. Apply strong to the resulting range [openIdx, openIdx+len(inner)).
	//
	// Step 3 needs an editor primitive we don't have (delete a range
	// at offset). For now we accept a narrower contract: marks earlier
	// in the buffer survive only if they don't intersect the opening
	// `**` markers. In practice, the user's typed `**foo**bar**baz**`
	// flow clears that bar — the opening markers are always plain
	// runes the user just typed, with no marks on them.
	//
	// Implementation: rebuild ONLY the prefix portion + apply marks to
	// inner. Pre-existing strong runs in prefix[:openIdx] survive
	// because we replay them via DeleteLastRune to the offset, then
	// re-insert (this is conservative but correct).
	editorLen := t.rich.CurrentParagraphLen()
	// Drop the trailing close `**` (2 runes).
	for i := 0; i < 2 && editorLen > 0; i++ {
		t.rich.DeleteLastRune()
		editorLen--
	}
	// Drop everything from the opening `**` to the end (inner.length + 2 runes).
	dropTail := len([]rune(inner)) + 2
	for i := 0; i < dropTail && editorLen > 0; i++ {
		t.rich.DeleteLastRune()
		editorLen--
	}
	// editorLen is now the length of prefix[:openIdx] (plain pre-bold).
	// Re-insert inner.
	innerStart := t.rich.CurrentParagraphLen()
	t.rich.InsertText(inner)
	innerEnd := t.rich.CurrentParagraphLen()
	t.rich.ApplyMarkToCurrentParagraphRange(innerStart, innerEnd, "strong")
}

// concatTextNodes returns all text content of an ADF doc concatenated.
func concatTextNodes(doc adf.Document) string {
	var b strings.Builder
	for _, block := range doc.Content {
		walkTextNodes(&b, block)
	}
	return b.String()
}

func walkTextNodes(b *strings.Builder, n adf.Node) {
	if n.Type == "text" {
		b.WriteString(n.Text)
	}
	for _, c := range n.Content {
		walkTextNodes(b, c)
	}
}

// isRichTextAction reports whether the input feeds a Jira rich-text
// field. These actions get an ADF document attached at submit; others
// pass the plain string through Value.
func isRichTextAction(action string) bool {
	switch action {
	case "comment":
		return true
	}
	return false
}

func (t TextInput) View() tea.View {
	if !t.Visible {
		return tea.NewView("")
	}
	prompt := lipgloss.NewStyle().Foreground(theme.ColorTitleFg).Bold(true).Render(t.prompt)
	hint := theme.HelpKey.Render("enter") + theme.HelpDesc.Render(" submit  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(" cancel")
	content := prompt + "\n\n" + t.input.View() + "\n\n" + hint
	return tea.NewView(RenderOverlay(content, 40))
}
