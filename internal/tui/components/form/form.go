package form

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	pkey "github.com/gechr/primer/key"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
)

// FieldSpec declares one field of a form.
type FieldSpec struct {
	// Label renders above the field; empty omits the row.
	Label string
	// Placeholder shows in the empty field.
	Placeholder string
	// Initial pre-fills the field; the dirty guard compares against it, so a
	// prefilled-then-untouched form still cancels without a confirmation.
	Initial string
	// Multiline makes the field a textarea (enter inserts a newline and
	// ctrl+s submits) instead of a one-line input.
	Multiline bool
	// Rows is the textarea height; zero means 5.
	Rows int
	// Optional lets the field submit blank. A required field left blank
	// blocks the submit and takes focus instead.
	Optional bool
	// Autocomplete, when set, watches this field for its trigger token and
	// offers fetched suggestions. Nil disables the seam.
	Autocomplete *Autocomplete
}

// Styles are the form's render styles, injected by the owner so the form
// stays theme-agnostic.
type Styles struct {
	Title              lipgloss.Style
	Label              lipgloss.Style
	LabelFocused       lipgloss.Style
	HintKey            lipgloss.Style
	HintText           lipgloss.Style
	Question           lipgloss.Style // the dirty-discard confirmation
	Suggestion         lipgloss.Style
	SuggestionSelected lipgloss.Style
}

// Config declares a whole form.
type Config struct {
	// Title renders above the fields (e.g. "comment on PROJ-1").
	Title  string
	Fields []FieldSpec
	// EditorHatch offers ctrl+e on a focused multiline field: the form emits
	// EventEditor and the owner takes the draft to an external editor. The
	// in-TUI field is always the default; the editor is the escape hatch.
	EditorHatch bool
	// Width bounds every field's rendered width.
	Width  int
	Styles Styles
}

// EventKind is what a completed Update asks the owner to do.
type EventKind int

const (
	// EventNone means the form consumed (or ignored) the message and stays open.
	EventNone EventKind = iota
	// EventSubmit means every required field is filled; read Values.
	EventSubmit
	// EventCancel means the user backed out (esc on a pristine form, or a
	// confirmed discard). The values are abandoned.
	EventCancel
	// EventEditor asks the owner to continue the focused multiline field in
	// the external editor, seeded with Values. The form stays open until the
	// owner closes it, so a failed editor launch loses nothing.
	EventEditor
)

// Model is one open form. The zero value is inert; construct with New.
type Model struct {
	title       string
	fields      []field
	focus       int
	confirming  bool
	editorHatch bool
	width       int
	styles      Styles
	active      bool
	ac          acState
}

// field pairs a spec with whichever input kind it declared.
type field struct {
	spec FieldSpec
	line input.Line
	area input.Area
}

// New builds a focused form: the first field owns the keyboard.
func New(cfg Config) Model {
	m := Model{
		title:       cfg.Title,
		editorHatch: cfg.EditorHatch,
		width:       cfg.Width,
		styles:      cfg.Styles,
		active:      len(cfg.Fields) > 0,
	}
	for i, spec := range cfg.Fields {
		f := field{spec: spec}
		if spec.Multiline {
			rows := spec.Rows
			if rows == 0 {
				rows = 5
			}
			f.area = input.NewArea(spec.Placeholder, cfg.Width, rows)
			f.area.SetValue(spec.Initial)
			if i != 0 {
				f.area.Blur()
			}
		} else {
			f.line = input.NewLine("", spec.Placeholder)
			f.line.SetWidth(cfg.Width)
			f.line.SetValue(spec.Initial)
			if i != 0 {
				f.line.Blur()
			}
		}
		m.fields = append(m.fields, f)
	}
	return m
}

// Active reports whether the form is open.
func (m *Model) Active() bool { return m.active }

// Value returns field i's current content ("" out of range).
func (m *Model) Value(i int) string {
	if i < 0 || i >= len(m.fields) {
		return ""
	}
	return m.fields[i].value()
}

// Values returns every field's current content in declaration order.
func (m *Model) Values() []string {
	out := make([]string, len(m.fields))
	for i := range m.fields {
		out[i] = m.fields[i].value()
	}
	return out
}

// dirty reports whether any field differs from its initial content — the
// gate on the discard confirmation.
func (m *Model) dirty() bool {
	for i := range m.fields {
		if m.fields[i].value() != m.fields[i].spec.Initial {
			return true
		}
	}
	return false
}

// Update routes a message into the form. The event tells the owner what
// completed; consumed reports whether the form owned the message, so an owner
// layering global keys knows when to fall through.
func (m *Model) Update(msg tea.Msg) (tea.Cmd, EventKind, bool) {
	if !m.active {
		return nil, EventNone, false
	}
	if sug, ok := msg.(SuggestionsMsg); ok {
		if m.confirming {
			// Dropping the result must also drop the query, or a resumed
			// form would see "same token" forever and never refetch.
			m.ac.clear()
			return nil, EventNone, true
		}
		m.applySuggestions(sug)
		return nil, EventNone, true
	}
	key, isKey := msg.(tea.KeyPressMsg)
	if m.confirming {
		if !isKey {
			// A paste (or any stray message) must not mutate the draft the
			// confirmation is protecting; only y/n/esc move the state.
			return nil, EventNone, true
		}
		return nil, m.updateConfirm(key), true
	}
	if !isKey {
		// Paste and ticks flow into the focused field; paste can change the
		// autocomplete token exactly like typing.
		cmd := m.fields[m.focus].update(msg)
		return tea.Batch(cmd, m.syncAutocomplete()), EventNone, true
	}
	if m.ac.visible() {
		if cmd, ev, done := m.updateSuggesting(key); done {
			return cmd, ev, true
		}
	}
	switch key.String() {
	case "esc":
		if m.dirty() {
			m.confirming = true
			return nil, EventNone, true
		}
		return nil, EventCancel, true
	case "ctrl+s":
		return nil, m.trySubmit(), true
	case "tab":
		m.moveFocus(1)
		return nil, EventNone, true
	case "shift+tab":
		m.moveFocus(-1)
		return nil, EventNone, true
	case "ctrl+e":
		if m.editorHatch && m.fields[m.focus].spec.Multiline {
			return nil, EventEditor, true
		}
	case "enter":
		// Enter on a one-line field advances to the next field (a newline is
		// meaningless there) and submits from the last one. Multiline fields
		// keep enter as a newline via the textarea below.
		if !m.fields[m.focus].spec.Multiline {
			if m.focus < len(m.fields)-1 {
				m.moveFocus(1)
				return nil, EventNone, true
			}
			return nil, m.trySubmit(), true
		}
	}
	cmd := m.fields[m.focus].update(msg)
	return tea.Batch(cmd, m.syncAutocomplete()), EventNone, true
}

// updateConfirm drives the discard confirmation: y abandons the form, n (or
// esc) resumes editing, anything else is swallowed so a stray key can never
// drop the draft.
func (m *Model) updateConfirm(key tea.KeyPressMsg) EventKind {
	switch key.String() {
	case "y", "Y":
		return EventCancel
	case "n", "N", "esc":
		m.confirming = false
	}
	return EventNone
}

// trySubmit emits EventSubmit when every required field is filled; otherwise
// it focuses the first blank required field and keeps the form open.
func (m *Model) trySubmit() EventKind {
	for i := range m.fields {
		if !m.fields[i].spec.Optional && xstrings.IsBlank(m.fields[i].value()) {
			m.setFocus(i)
			return EventNone
		}
	}
	return EventSubmit
}

// moveFocus advances the ring by delta, wrapping at both ends.
func (m *Model) moveFocus(delta int) {
	if len(m.fields) < 2 {
		return
	}
	m.setFocus((m.focus + delta + len(m.fields)) % len(m.fields))
}

func (m *Model) setFocus(i int) {
	if i == m.focus {
		return
	}
	m.fields[m.focus].blur()
	m.focus = i
	m.fields[i].focus()
	m.ac.clear() // suggestions belong to the field that was being typed in
}

// View renders the form content — title, labeled fields, any suggestion list
// under the focused field, and the hint row. The owner draws the box and
// places it.
func (m *Model) View() string {
	if !m.active {
		return ""
	}
	var b strings.Builder
	if m.title != "" {
		b.WriteString(m.styles.Title.Render(m.title))
		b.WriteString("\n")
	}
	for i := range m.fields {
		f := &m.fields[i]
		if f.spec.Label != "" {
			st := m.styles.Label
			if i == m.focus {
				st = m.styles.LabelFocused
			}
			b.WriteString(st.Render(f.spec.Label))
			b.WriteString("\n")
		}
		b.WriteString(f.view())
		b.WriteString("\n")
		if i == m.focus && m.ac.visible() {
			b.WriteString(m.viewSuggestions())
		}
	}
	b.WriteString(m.hintRow())
	return b.String()
}

// hintRow is the bottom line: the discard question while confirming, the
// suggestion bindings while the list is up, else the form's own bindings.
// Single-letter keys render inline in their description (primer's key.Inline
// "(n)ew" style); multi-letter keys fall back to "key desc".
func (m *Model) hintRow() string {
	if m.confirming {
		return m.styles.Question.Render("discard input?") + m.renderHints([]pkey.Hint{
			{Key: "y", Desc: "yes"},
			{Key: "n", Desc: "no"},
		})
	}
	if m.ac.visible() {
		return m.renderHints([]pkey.Hint{
			{Key: "↑↓", Desc: "choose"},
			{Key: "enter", Desc: "insert"},
			{Key: "esc", Desc: "dismiss"},
		})
	}
	hints := make([]pkey.Hint, 0, 4)
	if m.multiline() {
		hints = append(hints, pkey.Hint{Key: "ctrl+s", Desc: "submit"})
	} else {
		hints = append(hints, pkey.Hint{Key: "enter", Desc: "submit"})
	}
	if len(m.fields) > 1 {
		hints = append(hints, pkey.Hint{Key: "tab", Desc: "next field"})
	}
	if m.editorHatch {
		hints = append(hints, pkey.Hint{Key: "ctrl+e", Desc: "editor"})
	}
	hints = append(hints, pkey.Hint{Key: "esc", Desc: "cancel"})
	return m.renderHints(hints)
}

func (m *Model) renderHints(hints []pkey.Hint) string {
	prefix := " "
	return pkey.Renderer{
		Styles: pkey.Styles{Key: m.styles.HintKey, Text: m.styles.HintText},
		Prefix: &prefix,
		Inline: true,
	}.Render(hints)
}

// multiline reports whether any field is a textarea, which moves submit from
// enter to ctrl+s.
func (m *Model) multiline() bool {
	for i := range m.fields {
		if m.fields[i].spec.Multiline {
			return true
		}
	}
	return false
}

func (f *field) value() string {
	if f.spec.Multiline {
		return f.area.Value()
	}
	return f.line.Value()
}

func (f *field) focus() {
	if f.spec.Multiline {
		f.area.Focus()
		return
	}
	f.line.Focus()
}

func (f *field) blur() {
	if f.spec.Multiline {
		f.area.Blur()
		return
	}
	f.line.Blur()
}

func (f *field) update(msg tea.Msg) tea.Cmd {
	if f.spec.Multiline {
		return f.area.Update(msg)
	}
	return f.line.Update(msg)
}

func (f *field) view() string {
	if f.spec.Multiline {
		return f.area.View()
	}
	return f.line.View()
}

func (f *field) beforeCursor() string {
	if f.spec.Multiline {
		return f.area.BeforeCursor()
	}
	return f.line.BeforeCursor()
}

func (f *field) replaceBeforeCursor(n int, s string) {
	if f.spec.Multiline {
		f.area.ReplaceBeforeCursor(n, s)
		return
	}
	f.line.ReplaceBeforeCursor(n, s)
}
