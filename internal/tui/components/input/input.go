package input

import (
	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
)

// Line is a single-line input. The zero value is not usable; construct with
// NewLine.
type Line struct {
	ti textinput.Model
}

// NewLine builds a focused single-line input with the given prompt and
// placeholder.
func NewLine(prompt, placeholder string) Line {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.Focus()
	return Line{ti: ti}
}

// SetValue replaces the content and moves the cursor to the end (the natural
// spot when prefilling, e.g. the current summary for an edit).
func (l *Line) SetValue(s string) {
	l.ti.SetValue(s)
	l.ti.CursorEnd()
}

// Value returns the current content.
func (l Line) Value() string { return l.ti.Value() }

// SetWidth bounds the rendered width, clamped to at least one column so a
// too-narrow pane can never push a negative width into the textinput (which
// panics on View).
func (l *Line) SetWidth(w int) {
	if w < 1 {
		w = 1
	}
	l.ti.SetWidth(w)
}

// SetSuggestions enables ghost-text autocompletion over the given values:
// typing a prefix shows the rest faint, and the textinput's accept binding
// (tab / ctrl+e / right at end) completes it.
func (l *Line) SetSuggestions(values []string) {
	l.ti.ShowSuggestions = len(values) > 0
	l.ti.SetSuggestions(values)
}

// Suggestions returns the current completion values.
func (l Line) Suggestions() []string { return l.ti.AvailableSuggestions() }

// Update routes a message (keys, paste) into the input.
func (l *Line) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.ti, cmd = l.ti.Update(msg)
	return cmd
}

// View renders the input with its cursor.
func (l Line) View() string { return l.ti.View() }

// Area is a multiline input, the in-TUI fallback when no external editor is
// configured. The zero value is not usable; construct with NewArea.
type Area struct {
	ta textarea.Model
}

// NewArea builds a focused multiline input sized for a modal.
func NewArea(placeholder string, width, height int) Area {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.SetWidth(width)
	ta.SetHeight(height)
	ta.Focus()
	return Area{ta: ta}
}

// SetValue replaces the content (e.g. reopening a draft).
func (a *Area) SetValue(s string) { a.ta.SetValue(s) }

// Value returns the current content.
func (a Area) Value() string { return a.ta.Value() }

// Update routes a message into the textarea (enter inserts a newline).
func (a *Area) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	a.ta, cmd = a.ta.Update(msg)
	return cmd
}

// View renders the textarea.
func (a Area) View() string { return a.ta.View() }
