package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
)

// openJumplist opens the recent-issues picker (most recent first). Issue
// keys are a safe charset, so they ride the picker values directly.
func (r *results) openJumplist() {
	recent := r.ctx.Recent.List()
	items := make([]picker.Item, len(recent))
	for i, e := range recent {
		items[i] = picker.Item{Label: e.Key + "  " + e.Summary, Value: e.Key}
	}
	r.jumpPick = picker.New("Recent issues:", items)
	r.jumping = true
}

// updateJump drives the jumplist picker: enter jumps to the chosen issue —
// selecting it in the current list when present, otherwise opening its
// detail view directly (the detail fetch loads the full issue by key).
func (r *results) updateJump(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		r.jumping = false
		return nil
	case "enter":
		r.jumping = false
		sel, ok := r.jumpPick.Selected()
		if !ok {
			return nil
		}
		return r.jumpTo(sel.Value)
	default:
		return r.jumpPick.Update(msg)
	}
}

// jumpTo opens the issue's detail: in-list issues also move the cursor so
// closing the detail lands on the row; foreign keys (visited in another
// section or query) open from a stub that the detail fetch fills in.
func (r *results) jumpTo(key string) tea.Cmd {
	for i, iss := range r.shown {
		if issueKey(iss) == key {
			r.list.SetCursor(i)
			r.syncPreview()
			r.viewSelected()
			return r.openDetail(iss)
		}
	}
	k := key
	stub := &jira.Issue{Key: &k, Fields: &jira.IssueFields{}}
	return r.openDetail(stub)
}
