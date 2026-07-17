package picker

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gechr/primer/filter"

	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// Item is one selectable entry: Label is shown, Value is what the caller
// receives (e.g. a transition ID behind its display name).
type Item struct {
	Label string
	Value string
}

// Model is the picker state. Construct with New.
type Model struct {
	title   string
	items   []Item
	matches []int // indexes into items that pass the filter, in order
	cursor  int   // position within matches
	filter  input.Line
}

// New builds a picker over items with the cursor on the first entry and an
// empty (all-matching) filter.
func New(title string, items []Item) Model {
	m := Model{title: title, items: items, filter: input.NewLine("/ ", "filter")}
	m.refilter()
	return m
}

// Move shifts the cursor by delta within the current matches, clamped.
func (m *Model) Move(delta int) {
	m.cursor += delta
	m.clamp()
}

func (m *Model) clamp() {
	if len(m.matches) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
}

// refilter recomputes matches for the current filter text and snaps the
// cursor to the first match — a narrowed list under a stale cursor would
// submit something the user isn't looking at.
//
// Matching is fuzzy (primer/filter): query runes must appear in the label in
// order but need not be contiguous, so "ipr" now matches "In Progress". This
// broadens the match set versus the old substring test. CaseInsensitive keeps
// the picker's long-standing case-blind contract (an uppercased query still
// matches a lowercase label); smart case would have quietly narrowed it.
func (m *Model) refilter() {
	q := strings.TrimSpace(m.filter.Value())
	// A fresh slice, not a truncation: Model is copied by value, and two
	// copies sharing a backing array would stomp each other's matches.
	m.matches = nil
	// Case-insensitive substring through primer's filter, preserving the prior
	// strings.Contains behavior. Fuzzy subsequence matching is a deliberate
	// later change: it broadens matches unhelpfully over labels that embed a
	// JQL (the preset dropdown), so it is not adopted in this refactor.
	term := filter.Term{Text: q, Case: filter.CaseInsensitive}
	for i, it := range m.items {
		if q == "" || term.Match(it.Label) {
			m.matches = append(m.matches, i)
		}
	}
	m.cursor = 0
}

// Update routes navigation to the cursor and everything else (typing, paste,
// cursor movement inside the filter) to the filter input. Enter and esc are
// deliberately not handled — submit/cancel belong to the caller.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	// Arrow keys navigate; every printable key belongs to the filter (fzf
	// semantics), so letter aliases like j/k are deliberately not navigation
	// here. Matching on Code, not String(), keeps modified arrows working.
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.Code {
		case tea.KeyUp:
			m.Move(-1)
			return nil
		case tea.KeyDown:
			m.Move(1)
			return nil
		}
	}
	before := m.filter.Value()
	cmd := m.filter.Update(msg)
	if m.filter.Value() != before {
		m.refilter()
	}
	return cmd
}

// Selected returns the item under the cursor; ok is false when nothing
// matches the filter (or the picker is empty).
func (m Model) Selected() (Item, bool) {
	if len(m.matches) == 0 {
		return Item{}, false
	}
	return m.items[m.matches[m.cursor]], true
}

// View renders the title, the filter line (once the user typed something),
// and the matching items with a cursor marker.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title) + "\n")
	if m.filter.Value() != "" {
		b.WriteString(m.filter.View() + "\n")
	}
	if len(m.matches) == 0 {
		b.WriteString(theme.DetailDim.Render("(no match)"))
		return b.String()
	}
	for pos, idx := range m.matches {
		marker := "  "
		label := m.items[idx].Label
		if pos == m.cursor {
			marker = "▸ "
		} else {
			label = theme.DetailDim.Render(label)
		}
		b.WriteString(marker + label)
		if pos < len(m.matches)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
