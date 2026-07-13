// The autocomplete seam: trigger-token detection in the focused
// field, asynchronous suggestion fetches as commands, and the selection list
// rendered under the field.

package form

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Autocomplete plugs completion into one field. The form watches the word
// being typed at the cursor; once it starts with Trigger and the query is
// long enough, Fetch runs in a command and its results show as a list.
type Autocomplete struct {
	// Trigger starts a completion token at a word boundary, e.g. '@'.
	Trigger rune
	// MinQuery is how many runes must follow the trigger before fetching;
	// zero means 1, so a bare trigger never fires a fetch.
	MinQuery int
	// Fetch resolves a query to suggestions. It runs inside a tea.Cmd (its
	// own goroutine), so it may block on I/O; the result is dropped if the
	// query has moved on by the time it lands.
	Fetch func(query string) []string
}

// SuggestionsMsg carries fetched suggestions back into the form. Field and
// Query pin the result to the fetch that asked, so a slow response can never
// attach to a newer token (the generation-guard idea, keyed by content).
type SuggestionsMsg struct {
	Field int
	Query string
	Items []string
}

// acState is the live completion state for the focused field.
type acState struct {
	query  string // token text after the trigger; "" means inactive
	active bool
	items  []string
	cursor int
}

func (s *acState) clear() { *s = acState{} }

func (s *acState) visible() bool { return s.active && len(s.items) > 0 }

// maxSuggestions bounds the rendered list so a broad query can't grow the
// modal past its box.
const maxSuggestions = 5

// syncAutocomplete re-derives the trigger token after the focused field's
// content changed, returning a fetch command when the query is new.
func (m *Model) syncAutocomplete() tea.Cmd {
	ac := m.fields[m.focus].spec.Autocomplete
	if ac == nil {
		return nil
	}
	query, _, ok := triggerToken(m.fields[m.focus].beforeCursor(), ac.Trigger)
	minQuery := max(ac.MinQuery, 1)
	if !ok || len([]rune(query)) < minQuery {
		m.ac.clear()
		return nil
	}
	if m.ac.active && m.ac.query == query {
		return nil // same token — keep the list and cursor as they are
	}
	m.ac = acState{query: query, active: true}
	fieldIdx, fetch := m.focus, ac.Fetch
	if fetch == nil {
		return nil
	}
	return func() tea.Msg {
		return SuggestionsMsg{Field: fieldIdx, Query: query, Items: fetch(query)}
	}
}

// applySuggestions installs fetched items when they still match the live
// query; stale responses (field moved, token changed) drop silently.
func (m *Model) applySuggestions(msg SuggestionsMsg) {
	if msg.Field != m.focus || !m.ac.active || msg.Query != m.ac.query {
		return
	}
	items := msg.Items
	if len(items) > maxSuggestions {
		items = items[:maxSuggestions]
	}
	m.ac.items = items
	m.ac.cursor = 0
}

// updateSuggesting handles keys while the suggestion list is up. done reports
// the key was owned here; otherwise the caller's normal dispatch continues
// (typing keeps editing the field, which re-derives the token).
func (m *Model) updateSuggesting(key tea.KeyPressMsg) (tea.Cmd, EventKind, bool) {
	switch key.String() {
	case "up", "ctrl+p":
		if m.ac.cursor > 0 {
			m.ac.cursor--
		}
		return nil, EventNone, true
	case "down", "ctrl+n":
		if m.ac.cursor < len(m.ac.items)-1 {
			m.ac.cursor++
		}
		return nil, EventNone, true
	case "enter", "tab":
		m.acceptSuggestion()
		return nil, EventNone, true
	case "esc":
		// Dismissing the list is not backing out of the form; the guard only
		// sees the next esc.
		m.ac.clear()
		return nil, EventNone, true
	}
	return nil, EventNone, false
}

// acceptSuggestion replaces the trigger token with the selected item, keeping
// the trigger rune so the owner can recognize the mention when parsing.
func (m *Model) acceptSuggestion() {
	ac := m.fields[m.focus].spec.Autocomplete
	if ac == nil || !m.ac.visible() {
		return
	}
	item := m.ac.items[m.ac.cursor]
	_, width, ok := triggerToken(m.fields[m.focus].beforeCursor(), ac.Trigger)
	if !ok {
		m.ac.clear()
		return
	}
	m.fields[m.focus].replaceBeforeCursor(width, string(ac.Trigger)+item)
	m.ac.clear()
}

// viewSuggestions renders the list under the focused field, selection styled.
func (m *Model) viewSuggestions() string {
	var b strings.Builder
	for i, item := range m.ac.items {
		st := m.styles.Suggestion
		if i == m.ac.cursor {
			st = m.styles.SuggestionSelected
		}
		b.WriteString(st.Render(" " + item))
		b.WriteString("\n")
	}
	return b.String()
}

// triggerToken finds a completion token in the text before the cursor: the
// trailing word must start with trigger. query is the text after the trigger;
// width is the whole token's rune count including the trigger, which is what
// an acceptance replaces.
func triggerToken(before string, trigger rune) (query string, width int, ok bool) {
	runes := []rune(before)
	start := len(runes)
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	word := runes[start:]
	if len(word) == 0 || word[0] != trigger {
		return "", 0, false
	}
	return string(word[1:]), len(word), true
}
