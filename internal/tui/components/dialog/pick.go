package dialog

import (
	tea "charm.land/bubbletea/v2"

	pkey "github.com/gechr/primer/key"

	"github.com/matcra587/jira-cli/internal/tui/components/picker"
)

// Pick is a filterable single-choice dialog: it wraps a picker.Model, letting
// the Shell frame and scroll a type-to-filter select list. Typing narrows the
// list and the arrows move the cursor — routed straight into the picker —
// while enter accepts the highlighted item (ResultSubmit) and esc cancels
// (ResultClose), the two keys the picker itself deliberately leaves to its
// caller.
//
// It carries pointer semantics: Update mutates and returns the same value, so
// the caller reads Selected off the very pointer the Stack pops.
type Pick struct {
	picker picker.Model
}

// NewPick returns a Pick over items, titled title and with the cursor on the
// first entry.
func NewPick(title string, items []picker.Item) *Pick {
	return &Pick{picker: picker.New(title, items)}
}

// Selected returns the highlighted item; ok is false when the filter matches
// nothing (or the picker is empty). It is meaningful once the Stack has popped
// the dialog with ResultSubmit.
func (p *Pick) Selected() (picker.Item, bool) { return p.picker.Selected() }

// Title omits the Shell's heading: the wrapped picker renders its own bold
// title as the first line of its View, so surfacing it here too would draw the
// title twice. This matches every other picker.Model consumer, which likewise
// lets the picker own its title.
func (p *Pick) Title() string { return "" }

// Content renders the picker — title, filter line, and matching rows — as the
// body. The picker does not wrap to a width, so the parameter is unused; the
// Shell frames and scrolls whatever it returns.
func (p *Pick) Content(int) string { return p.picker.View() }

// Hints advertises the accept and cancel keys for the Shell's foot row.
func (p *Pick) Hints() []pkey.Hint {
	return []pkey.Hint{
		{Key: "enter", Desc: "select"},
		{Key: "esc", Desc: "cancel"},
	}
}

// Update resolves the dialog on enter (accept) or esc (cancel) and routes
// everything else — filter typing, paste, arrow navigation — into the picker.
// Matching on Code, not String, keeps a modified enter or escape working and
// mirrors the picker's own key handling.
func (p *Pick) Update(msg tea.Msg) (Dialog, tea.Cmd, Result) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.Code {
		case tea.KeyEnter:
			return p, nil, ResultSubmit
		case tea.KeyEscape:
			return p, nil, ResultClose
		}
	}
	return p, p.picker.Update(msg), ResultNone
}
