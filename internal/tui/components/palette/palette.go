package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gechr/primer/filter"
	"github.com/gechr/primer/prompt"
)

// Entry is one palette command. Key is the literal keybinding the caller
// replays when this entry is chosen (e.g. "t", "c", "tab") — the palette never
// executes anything itself, so an invocation can never drift from the keyboard.
type Entry struct {
	Name string // command name shown first, e.g. "transition"
	Desc string // one-line description
	Key  string // keybinding to replay on selection
}

// Styles are injected so the widget stays theme-agnostic (like picker.Styles).
type Styles struct {
	Title    lipgloss.Style
	Query    lipgloss.Style // the typed filter text / prompt
	Name     lipgloss.Style
	Match    lipgloss.Style // fuzzy-matched runes within the name
	Desc     lipgloss.Style
	Selected lipgloss.Style // the highlighted row
	KeyHint  lipgloss.Style // the entry's replay key, shown at the row end
}

// match is one entry admitted by the filter, with the rune indexes its Name
// matched on — computed once per query edit and reused by every View, rather
// than re-running the fuzzy match per row per render.
type match struct {
	entry int   // index into entries
	hits  []int // rune indexes of the query's fuzzy match within the Name
}

// Model is a filterable command list. Zero value is unusable; construct with New.
type Model struct {
	title   string
	entries []Entry
	matches []match // entries that pass the filter, in input order
	cursor  int     // position within matches
	query   string
	styles  Styles
}

// New builds a palette over entries, titled title, cursor on the first match
// and an empty (all-matching) filter.
func New(title string, entries []Entry, styles Styles) Model {
	m := Model{title: title, entries: entries, styles: styles}
	m.refilter()
	return m
}

// refilter recomputes matches for the current query and snaps the cursor to
// the first match — a narrowed list under a stale cursor would accept a
// command the user isn't looking at. An empty query matches every entry in
// input order (Fuzzy returns an empty, non-nil index list); otherwise an entry
// matches when the query is a fuzzy subsequence of its Name. CaseInsensitive
// keeps the filter case-blind so an uppercased query still matches a lowercase
// name. The match indexes are kept for View's highlighting.
func (m *Model) refilter() {
	// A fresh slice, not a truncation: Model is copied by value, and two copies
	// sharing a backing array would stomp each other's matches.
	m.matches = nil
	for i, e := range m.entries {
		if hits := filter.Fuzzy(e.Name, m.query, filter.CaseInsensitive); hits != nil {
			m.matches = append(m.matches, match{entry: i, hits: hits})
		}
	}
	m.cursor = 0
}

// move shifts the cursor by delta within the current matches, clamped — no
// wrap, mirroring picker's choice so the two widgets navigate identically.
func (m *Model) move(delta int) {
	m.cursor = prompt.StepChoice(m.cursor, len(m.matches), delta, false)
}

// Update handles filter typing (runes, backspace) and cursor movement
// (up/ctrl+p, down/ctrl+n). It deliberately does NOT handle enter or esc: the
// caller (a dialog wrapper) owns accept and cancel, exactly like picker. A
// palette that swallowed enter could accept a command its wrapper meant to
// veto, and one that swallowed esc could trap a user who wanted out — keeping
// both keys with the caller makes the accept/cancel contract single-owned.
// Returns a cmd (currently always nil) and is a no-op for messages it does not
// recognize.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	// Match on String(): a typed rune reports its text (so it falls through to
	// the append below), while navigation and editing keys report their stable
	// names ("up", "ctrl+n", "backspace").
	switch key.String() {
	case "up", "ctrl+p":
		m.move(-1)
		return nil
	case "down", "ctrl+n":
		m.move(1)
		return nil
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
			m.refilter()
		}
		return nil
	}
	// Any key carrying printable text extends the query; the refilter resets
	// the cursor to the first match.
	if key.Text != "" {
		m.query += key.Text
		m.refilter()
	}
	return nil
}

// Selected returns the entry under the cursor; ok is false when the filter
// matches nothing (or the palette is empty). Meaningful after the caller
// decides to accept.
func (m Model) Selected() (Entry, bool) {
	if len(m.matches) == 0 {
		return Entry{}, false
	}
	return m.entries[m.matches[m.cursor].entry], true
}

// Query returns the current filter text, for tests and callers that echo it.
func (m Model) Query() string { return m.query }

// CursorLine returns the zero-based line offset of the selected row within
// View(), so a scrolling container can keep the selection in view. The layout
// is title, query, then one line per match, and the rows are short
// single-line commands (no wrapping), so the cursor sits two lines below the
// top plus its position among the matches. Zero when nothing matches.
func (m Model) CursorLine() int {
	if len(m.matches) == 0 {
		return 0
	}
	return 2 + m.cursor
}

// View renders the title, the query line, and the filtered rows. Each row is
// "name  — desc" with the fuzzy-matched runes styled via Match, the replay key
// styled via KeyHint at the row end, and the cursor row wrapped in Selected.
// Rows appear in input order among matches (a stable filter); non-matches are
// hidden. An empty match set renders a muted placeholder line.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render(m.title) + "\n")
	b.WriteString(m.styles.Query.Render("/ "+m.query) + "\n")

	if len(m.matches) == 0 {
		b.WriteString(m.styles.Desc.Render("no commands match"))
		return b.String()
	}

	for pos, mt := range m.matches {
		row := m.renderRow(m.entries[mt.entry], mt.hits)
		if pos == m.cursor {
			row = m.styles.Selected.Render(row)
		}
		b.WriteString(row)
		if pos < len(m.matches)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// renderRow assembles one command row: the highlighted name, the dimmed
// description, and the replay key hint at the end.
func (m Model) renderRow(e Entry, hits []int) string {
	var b strings.Builder
	b.WriteString(m.highlightName(e.Name, hits))
	if e.Desc != "" {
		b.WriteString("  " + m.styles.Desc.Render("— "+e.Desc))
	}
	if e.Key != "" {
		b.WriteString("  " + m.styles.KeyHint.Render(e.Key))
	}
	return b.String()
}

// highlightName styles the runes of name the filter matched with Match and the
// rest with Name, from the indexes refilter stored when it admitted the row.
// Contiguous matched runes render as one styled run rather than one ANSI pair
// per character. An empty query (no indexes) renders the whole name in Name.
func (m Model) highlightName(name string, hits []int) string {
	ranges := filter.FuzzyBytes(name, hits)
	if len(ranges) == 0 {
		return m.styles.Name.Render(name)
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(ranges); {
		start, end := ranges[i][0], ranges[i][1]
		j := i + 1
		for j < len(ranges) && ranges[j][0] == end {
			end = ranges[j][1]
			j++
		}
		if start > last {
			b.WriteString(m.styles.Name.Render(name[last:start]))
		}
		b.WriteString(m.styles.Match.Render(name[start:end]))
		last = end
		i = j
	}
	if last < len(name) {
		b.WriteString(m.styles.Name.Render(name[last:]))
	}
	return b.String()
}
