package picker

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func items(labels ...string) []Item {
	out := make([]Item, len(labels))
	for i, l := range labels {
		out[i] = Item{Label: l, Value: "id-" + l}
	}
	return out
}

func press(m *Model, text string) {
	m.Update(tea.KeyPressMsg{Text: text})
}

// matchedLabels returns the labels of the currently filtered rows, in order —
// a readable stand-in for the unexported matches slice.
func matchedLabels(m Model) []string {
	out := make([]string, 0, len(m.matches))
	for _, idx := range m.matches {
		out = append(out, m.items[idx].Label)
	}
	return out
}

func TestSelectedDefaultsToFirstItem(t *testing.T) {
	m := New("Transition to", items("To Do", "In Progress", "Done"))
	sel, ok := m.Selected()
	if !ok || sel.Label != "To Do" {
		t.Fatalf("Selected() = %+v, %v; want first item", sel, ok)
	}
}

func TestMoveClampsAtBounds(t *testing.T) {
	m := New("t", items("a", "b"))
	m.Move(-5)
	if sel, _ := m.Selected(); sel.Label != "a" {
		t.Errorf("underflow landed on %q", sel.Label)
	}
	m.Move(5)
	if sel, _ := m.Selected(); sel.Label != "b" {
		t.Errorf("overflow landed on %q", sel.Label)
	}
}

func TestUpDownKeysNavigate(t *testing.T) {
	m := New("t", items("a", "b", "c"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if sel, _ := m.Selected(); sel.Label != "c" {
		t.Errorf("after two downs Selected = %q, want c", sel.Label)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if sel, _ := m.Selected(); sel.Label != "b" {
		t.Errorf("after up Selected = %q, want b", sel.Label)
	}
}

func TestTypingFiltersItems(t *testing.T) {
	m := New("t", items("To Do", "In Progress", "Done"))
	press(&m, "pro")
	sel, ok := m.Selected()
	if !ok || sel.Label != "In Progress" {
		t.Fatalf("filter 'pro' Selected = %+v, %v; want In Progress", sel, ok)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "To Do") || strings.Contains(view, "Done") {
		t.Errorf("filtered-out items still visible:\n%s", view)
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	m := New("t", items("Done"))
	press(&m, "DONE")
	if _, ok := m.Selected(); !ok {
		t.Error("case-insensitive match failed")
	}
}

func TestNoMatchMeansNoSelection(t *testing.T) {
	m := New("t", items("a", "b"))
	press(&m, "zzz")
	if _, ok := m.Selected(); ok {
		t.Error("Selected() reported ok with nothing matching")
	}
	if !strings.Contains(ansi.Strip(m.View()), "no match") {
		t.Errorf("view should say no match:\n%s", m.View())
	}
}

func TestFilterChangeResetsCursor(t *testing.T) {
	m := New("t", items("alpha", "beta", "gamma"))
	m.Move(2)      // cursor on gamma
	press(&m, "a") // all three match; cursor must snap back to the first
	if sel, ok := m.Selected(); !ok || sel.Label != "alpha" {
		t.Errorf("cursor should reset to first match, got %+v", sel)
	}
}

func TestBackspaceWidensFilter(t *testing.T) {
	m := New("t", items("Done", "Do not"))
	press(&m, "done")
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	// "don" matches only "Done"; one more backspace → "do" matches both
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Do not") {
		t.Errorf("widened filter should re-show items:\n%s", view)
	}
}

func TestViewMarksCursorRowAndTitle(t *testing.T) {
	m := New("Transition to", items("a", "b"))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Transition to") {
		t.Errorf("title missing:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	marked := 0
	for _, l := range lines {
		if strings.Contains(l, "▸") {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("cursor marker count = %d, want exactly 1:\n%s", marked, view)
	}
}

func TestEmptyPickerNeverSelects(t *testing.T) {
	m := New("t", nil)
	if _, ok := m.Selected(); ok {
		t.Error("empty picker reported a selection")
	}
	m.Move(1) // must not panic
	_ = m.View()
}

func TestPasteFiltersToo(t *testing.T) {
	m := New("t", items("alpha", "beta"))
	m.Update(tea.PasteMsg{Content: "bet"})
	if sel, ok := m.Selected(); !ok || sel.Label != "beta" {
		t.Errorf("paste filter Selected = %+v, %v; want beta", sel, ok)
	}
}

// TestFuzzyMatching pins the primer/filter behavior: query runes match in
// order without needing to be contiguous, which broadens the match set beyond
// the old substring test while staying case-insensitive.
func TestSubstringMatching(t *testing.T) {
	all := []string{"To Do", "In Progress", "In Review", "Done"}
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"contiguous substring matches", "prog", []string{"In Progress"}},
		{"substring spanning both rows", "in", []string{"In Progress", "In Review"}},
		{"uppercase query stays case-insensitive", "DONE", []string{"Done"}},
		{"non-contiguous subsequence does not match", "ipr", nil},
		{"absent substring matches nothing", "zzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("t", items(all...))
			press(&m, tt.query)
			if got := matchedLabels(m); !slices.Equal(got, tt.want) {
				t.Errorf("query %q matched %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}
