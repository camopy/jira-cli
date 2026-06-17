package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/core"
)

func jumpModel(t *testing.T) *Model {
	t.Helper()
	m := changeModel(t)
	land(
		m,
		mkIssue("JCT-1", "To Do", "first"),
		mkIssue("JCT-2", "In Progress", "second"),
		mkIssue("JCT-3", "Done", "third"),
	)
	return m
}

func ctrlO() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl} }

func TestOpenDetailRecordsRecentVisit(t *testing.T) {
	m := jumpModel(t)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open detail on JCT-1
	got := m.ctx.Recent.List()
	if len(got) != 1 || got[0].Key != "JCT-1" || got[0].Summary != "first" {
		t.Fatalf("recent after open = %+v, want [JCT-1 first]", got)
	}
}

func TestCtrlOOpensJumplistAndCapturesInput(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("JCT-2", "second")
	m.Update(ctrlO())
	if !m.jumping || !m.CapturesInput() {
		t.Fatal("ctrl+o did not open the jumplist")
	}
}

func TestJumpToInListIssueSelectsRowAndOpensDetail(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("JCT-1", "first")
	m.ctx.Recent.Touch("JCT-3", "third") // most recent → picker top
	m.Update(ctrlO())
	// Type to narrow to JCT-1, then jump.
	for _, r := range "jct-1" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.jumping {
		t.Fatal("enter did not close the jumplist")
	}
	if !m.detailing || issueKey(m.detailIssue) != "JCT-1" {
		t.Fatalf("detail after jump = detailing=%v issue=%v", m.detailing, m.detailIssue)
	}
	if sel := m.selected(); sel == nil || issueKey(sel) != "JCT-1" {
		t.Errorf("cursor did not land on JCT-1: %v", sel)
	}
}

func TestJumpToForeignKeyOpensStubDetail(t *testing.T) {
	m := jumpModel(t)
	// Visited in another section/query: not in this list.
	m.ctx.Recent.Touch("OPS-9", "elsewhere")
	m.Update(ctrlO())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.detailing || issueKey(m.detailIssue) != "OPS-9" {
		t.Fatalf("foreign jump = detailing=%v issue=%v, want OPS-9 stub", m.detailing, m.detailIssue)
	}
	if got := m.ctx.Recent.List()[0].Key; got != "OPS-9" {
		t.Errorf("recent front after foreign jump = %s", got)
	}
}

func TestJumplistEscClosesWithoutJumping(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("JCT-2", "second")
	m.Update(ctrlO())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.jumping || m.detailing {
		t.Errorf("esc left jumping=%v detailing=%v", m.jumping, m.detailing)
	}
}

func TestPasteTypesIntoOpenJumplist(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("JCT-1", "first")
	m.ctx.Recent.Touch("JCT-2", "second")
	m.Update(ctrlO())
	m.Update(tea.PasteMsg{Content: "jct-1"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.detailing || issueKey(m.detailIssue) != "JCT-1" {
		t.Errorf("pasted filter jump = detailing=%v issue=%v", m.detailing, m.detailIssue)
	}
}

func TestClickAndWheelIgnoredUnderJumplist(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("JCT-2", "second")
	m.Update(ctrlO())
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: m.listTop()})
	if m.detailing || m.list.Cursor() != 0 {
		t.Errorf("click under jumplist acted: detailing=%v cursor=%d", m.detailing, m.list.Cursor())
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: m.listTop()})
	if m.list.Cursor() != 0 {
		t.Errorf("wheel under jumplist moved cursor to %d", m.list.Cursor())
	}
	if !m.jumping {
		t.Error("mouse events closed the jumplist")
	}
}

func TestDetailFetchRefreshesRecentSummary(t *testing.T) {
	m := jumpModel(t)
	m.ctx.Recent.Touch("OPS-9", "old name")
	m.Update(ctrlO())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // foreign jump via stub
	m.handleTask(core.TaskFinishedMsg{
		Scope:  m.detailScope(),
		Result: detailResult{issue: mkIssue("OPS-9", "To Do", "renamed upstream")},
	})
	if got := m.ctx.Recent.List()[0]; got.Key != "OPS-9" || got.Summary != "renamed upstream" {
		t.Errorf("recent after detail fetch = %+v, want refreshed summary", got)
	}
}

func TestRecentDedupesAcrossRepeatOpens(t *testing.T) {
	m := jumpModel(t)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open JCT-1
	m.handleTask(core.TaskFinishedMsg{Scope: m.detailScope(), Result: detailResult{issue: m.detailIssue}})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})     // close detail
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // down to JCT-2
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // open JCT-2
	m.handleTask(core.TaskFinishedMsg{Scope: m.detailScope(), Result: detailResult{issue: m.detailIssue}})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"}) // back up
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // reopen JCT-1
	got := m.ctx.Recent.List()
	if len(got) != 2 || got[0].Key != "JCT-1" || got[1].Key != "JCT-2" {
		t.Errorf("recent order = %+v, want [JCT-1 JCT-2]", got)
	}
}
