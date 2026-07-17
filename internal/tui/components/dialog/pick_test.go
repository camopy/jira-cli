package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/matcra587/jira-cli/internal/tui/components/picker"
)

func pickItems() []picker.Item {
	return []picker.Item{
		{Label: "alpha", Value: "a"},
		{Label: "beta", Value: "b"},
		{Label: "gamma", Value: "c"},
	}
}

func TestPick(t *testing.T) {
	t.Run("open shows the title and every item", func(t *testing.T) {
		p := NewPick("choose one", pickItems())
		view := ansi.Strip(p.Content(40))
		for _, want := range []string{"choose one", "alpha", "beta", "gamma"} {
			if !strings.Contains(view, want) {
				t.Fatalf("view missing %q:\n%s", want, view)
			}
		}
	})

	t.Run("the title is not doubled by the shell", func(t *testing.T) {
		// The picker draws its own title, so Pick leaves the Shell's heading
		// empty; the two together must not render the title twice.
		if got := NewPick("choose one", pickItems()).Title(); got != "" {
			t.Fatalf("Title() = %q, want empty so the picker's own title is not doubled", got)
		}
	})

	t.Run("filter narrows to the match", func(t *testing.T) {
		p := NewPick("choose one", pickItems())
		_, _, res := p.Update(tea.KeyPressMsg{Text: "bet"})
		if res != ResultNone {
			t.Fatalf("typing resolved the dialog: res = %v", res)
		}
		view := ansi.Strip(p.Content(40))
		if !strings.Contains(view, "beta") {
			t.Fatalf("filtered view lost the match:\n%s", view)
		}
		if strings.Contains(view, "alpha") || strings.Contains(view, "gamma") {
			t.Fatalf("filter did not narrow away non-matches:\n%s", view)
		}
		if sel, ok := p.Selected(); !ok || sel.Value != "b" {
			t.Fatalf("Selected() = %+v, %v; want beta", sel, ok)
		}
	})

	t.Run("enter submits the highlighted item", func(t *testing.T) {
		p := NewPick("choose one", pickItems())
		// Move the cursor down one so the choice is not just the default first
		// row, proving the submission reflects navigation.
		p.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		next, _, res := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if res != ResultSubmit {
			t.Fatalf("enter result = %v, want ResultSubmit", res)
		}
		if next.(*Pick) != p {
			t.Fatal("Update returned a different value than the receiver")
		}
		if sel, ok := p.Selected(); !ok || sel.Value != "b" {
			t.Fatalf("Selected() after enter = %+v, %v; want beta", sel, ok)
		}
	})

	t.Run("esc closes without a selection commitment", func(t *testing.T) {
		p := NewPick("choose one", pickItems())
		_, _, res := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if res != ResultClose {
			t.Fatalf("esc result = %v, want ResultClose", res)
		}
	})
}
