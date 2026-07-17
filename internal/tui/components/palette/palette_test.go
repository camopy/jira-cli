package palette

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// entries builds a palette entry per name; Desc and Key are derived so a row
// carries all three fields without the tests spelling them out.
func entries(names ...string) []Entry {
	out := make([]Entry, len(names))
	for i, n := range names {
		out[i] = Entry{Name: n, Desc: n + " desc", Key: n[:1]}
	}
	return out
}

// press feeds text to the palette as a single key press, the same shape the
// input substrate delivers typed runes.
func press(m *Model, text string) {
	m.Update(tea.KeyPressMsg{Text: text})
}

// matchedNames returns the names of the currently visible rows, in order — a
// readable stand-in for the unexported matches slice.
func matchedNames(m Model) []string {
	out := make([]string, 0, len(m.matches))
	for _, mt := range m.matches {
		out = append(out, m.entries[mt.entry].Name)
	}
	return out
}

func TestCursorLineTracksSelection(t *testing.T) {
	m := New("Commands", entries("transition", "comment", "assign"), Styles{})
	// Title and query occupy the first two lines, so the first row sits at 2.
	if got := m.CursorLine(); got != 2 {
		t.Errorf("initial CursorLine = %d, want 2", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.CursorLine(); got != 4 {
		t.Errorf("CursorLine after two downs = %d, want 4", got)
	}
	// A query that matches nothing has no row to point at.
	press(&m, "zzz")
	if got := m.CursorLine(); got != 0 {
		t.Errorf("CursorLine with no matches = %d, want 0", got)
	}
}

func TestEmptyQueryShowsAll(t *testing.T) {
	m := New("Commands", entries("transition", "comment", "assign"), Styles{})
	if got, want := matchedNames(m), []string{"transition", "comment", "assign"}; !slices.Equal(got, want) {
		t.Errorf("empty query rows = %v, want %v", got, want)
	}
	if sel, ok := m.Selected(); !ok || sel.Name != "transition" {
		t.Errorf("Selected = %+v, %v; want first entry", sel, ok)
	}
}

func TestFuzzyQueryNarrows(t *testing.T) {
	all := []string{"transition", "comment", "assign", "attach"}
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"contiguous prefix", "tran", []string{"transition"}},
		{"non-contiguous subsequence", "tsn", []string{"transition"}},
		{"shared subsequence keeps input order", "a", []string{"transition", "assign", "attach"}},
		{"uppercase stays case-insensitive", "COMMENT", []string{"comment"}},
		{"no subsequence matches nothing", "zzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("t", entries(all...), Styles{})
			press(&m, tt.query)
			if got := matchedNames(m); !slices.Equal(got, tt.want) {
				t.Errorf("query %q matched %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCursorUpDownClamps(t *testing.T) {
	m := New("t", entries("one", "two", "three"), Styles{})

	// Down past the end clamps on the last entry.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if sel, _ := m.Selected(); sel.Name != "three" {
		t.Errorf("down overflow landed on %q, want three", sel.Name)
	}

	// Up past the start clamps on the first entry.
	for range 5 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if sel, _ := m.Selected(); sel.Name != "one" {
		t.Errorf("up underflow landed on %q, want one", sel.Name)
	}
}

func TestCtrlNCtrlPNavigate(t *testing.T) {
	m := New("t", entries("one", "two", "three"), Styles{})
	m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'n'})
	if sel, _ := m.Selected(); sel.Name != "two" {
		t.Errorf("ctrl+n landed on %q, want two", sel.Name)
	}
	m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'p'})
	if sel, _ := m.Selected(); sel.Name != "one" {
		t.Errorf("ctrl+p landed on %q, want one", sel.Name)
	}
}

func TestBackspaceRestoresQuery(t *testing.T) {
	m := New("t", entries("comment", "commit"), Styles{})
	press(&m, "comme") // only "comment" is a subsequence
	if got := matchedNames(m); !slices.Equal(got, []string{"comment"}) {
		t.Fatalf("after typing rows = %v, want [comment]", got)
	}
	// Backspace twice widens "comme" → "com", which both entries match.
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.Query() != "com" {
		t.Errorf("query after two backspaces = %q, want com", m.Query())
	}
	if got := matchedNames(m); !slices.Equal(got, []string{"comment", "commit"}) {
		t.Errorf("widened rows = %v, want both", got)
	}
}

func TestSelectedTracksCursorAndReportsEmpty(t *testing.T) {
	m := New("t", entries("transition", "comment"), Styles{})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	sel, ok := m.Selected()
	if !ok || sel.Name != "comment" || sel.Key != "c" {
		t.Errorf("Selected = %+v, %v; want the comment entry", sel, ok)
	}

	press(&m, "zzz")
	if _, ok := m.Selected(); ok {
		t.Error("Selected reported ok with nothing matching")
	}
	if !strings.Contains(ansi.Strip(m.View()), "no commands match") {
		t.Errorf("empty view should show the placeholder:\n%s", m.View())
	}
}

func TestQueryEchoesTypedText(t *testing.T) {
	m := New("t", entries("transition"), Styles{})
	if m.Query() != "" {
		t.Errorf("fresh query = %q, want empty", m.Query())
	}
	press(&m, "tr")
	press(&m, "an")
	if m.Query() != "tran" {
		t.Errorf("query = %q, want tran", m.Query())
	}
}
