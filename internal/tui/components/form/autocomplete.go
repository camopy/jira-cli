// The autocomplete seam: trigger-token detection in the focused
// field, asynchronous suggestion fetches as commands, and the selection list
// rendered under the field.

package form

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Suggestion is one completion candidate: Value is inserted into the field on
// acceptance (with the trigger rune re-prefixed when there is one), while Label
// is what the list renders. The two are equal for plain value lists; they
// diverge when a human-readable label stands in for an opaque value.
type Suggestion struct {
	Value string
	Label string
	// Detail is an opaque payload carried with the suggestion but never
	// inserted into the field — the owner reads it via AcceptedDetail after an
	// acceptance. It lets a readable label (an assignee's display name) stand
	// in the field while the owner keeps the value it maps to (an accountId),
	// so no re-resolve is needed at submit.
	Detail string
}

// Autocomplete plugs completion into one field. The form watches the word
// being typed at the cursor; once it starts with Trigger and the query is
// long enough, Fetch runs in a command and its results show as a list.
type Autocomplete struct {
	// Trigger starts a completion token at a word boundary, e.g. '@'. Zero
	// means bare mode: every trailing token completes and an acceptance
	// replaces the token as-is — for value lists rather than mentions.
	Trigger rune
	// MinQuery is how many runes must follow the trigger before fetching;
	// zero means 1, so a bare trigger never fires a fetch.
	MinQuery int
	// IsBoundary marks the runes that end a token; nil means whitespace.
	// A comma-separated field adds ',' so each list entry completes alone.
	IsBoundary func(r rune) bool
	// Fetch resolves a query to suggestions. It runs inside a tea.Cmd (its
	// own goroutine), so it may block on I/O; the result is dropped if the
	// query has moved on by the time it lands.
	Fetch func(query string) []Suggestion
}

// SuggestionsMsg carries fetched suggestions back into the form. Field and
// Query pin the result to the fetch that asked, so a slow response can never
// attach to a newer token (the generation-guard idea, keyed by content).
type SuggestionsMsg struct {
	Field int
	Query string
	Items []Suggestion
}

// acState is the live completion state for the focused field.
type acState struct {
	query  string // token text after the trigger; "" means inactive
	active bool
	items  []Suggestion
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
	query, _, ok := triggerToken(m.fields[m.focus].beforeCursor(), ac.Trigger, ac.IsBoundary)
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

// acceptSuggestion replaces the token with the selected item — keeping the
// trigger rune when there is one, so the owner can recognize the mention
// when parsing; bare mode swaps the token as-is.
func (m *Model) acceptSuggestion() {
	ac := m.fields[m.focus].spec.Autocomplete
	if ac == nil || !m.ac.visible() {
		return
	}
	chosen := m.ac.items[m.ac.cursor]
	item := chosen.Value
	_, width, ok := triggerToken(m.fields[m.focus].beforeCursor(), ac.Trigger, ac.IsBoundary)
	if !ok {
		m.ac.clear()
		return
	}
	if ac.Trigger != 0 {
		item = string(ac.Trigger) + item
	}
	m.fields[m.focus].replaceBeforeCursor(width, item)
	// Record the accepted detail on this (main-loop) goroutine so the owner can
	// read the accountId behind an assignee's display name without a re-resolve.
	// A later edit to the field clears it (see clearErrors), invalidating a
	// stale mapping.
	m.acceptedDetail[m.focus] = chosen.Detail
	m.ac.clear()
}

// viewSuggestions renders the list under the focused field, selection styled.
// The list shows each suggestion's Label; acceptance inserts its Value.
func (m *Model) viewSuggestions() string {
	var b strings.Builder
	for i, item := range m.ac.items {
		st := m.styles.Suggestion
		if i == m.ac.cursor {
			st = m.styles.SuggestionSelected
		}
		b.WriteString(st.Render(" " + item.Label))
		b.WriteString("\n")
	}
	return b.String()
}

// triggerToken finds a completion token in the text before the cursor: the
// trailing word (bounded by isBoundary, whitespace when nil) must start with
// trigger — or is taken whole in bare mode (trigger 0). query is the text
// after any trigger; width is the whole token's rune count including the
// trigger, which is what an acceptance replaces.
func triggerToken(before string, trigger rune, isBoundary func(rune) bool) (query string, width int, ok bool) {
	if isBoundary == nil {
		isBoundary = unicode.IsSpace
	}
	runes := []rune(before)
	start := len(runes)
	for start > 0 && !isBoundary(runes[start-1]) {
		start--
	}
	word := runes[start:]
	if trigger == 0 {
		if len(word) == 0 {
			return "", 0, false
		}
		return string(word), len(word), true
	}
	if len(word) == 0 || word[0] != trigger {
		return "", 0, false
	}
	return string(word[1:]), len(word), true
}
