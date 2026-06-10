package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// clickModel builds an issues section with rows and a rendered frame, so the
// click hit-tests see the same geometry the user does.
func clickModel(t *testing.T, n int) *Model {
	t.Helper()
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.Init(ctx)
	issues := make([]*jira.Issue, n)
	for i := range issues {
		issues[i] = mkIssue("JCT-"+string(rune('1'+i)), "To Do", "row")
	}
	m.all = issues
	m.applyFilter()
	_ = m.View() // caches headerRows for the hit-tests
	return m
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestClickSelectsRow(t *testing.T) {
	m := clickModel(t, 3)
	m.Update(click(2, m.listTop()+2))
	if got := m.list.Cursor(); got != 2 {
		t.Errorf("cursor after click = %d, want 2", got)
	}
}

func TestClickOnSelectedRowOpensDetail(t *testing.T) {
	m := clickModel(t, 3)
	m.Update(click(2, m.listTop()+1))
	if m.detailing {
		t.Fatal("first click must only select")
	}
	m.Update(click(2, m.listTop()+1))
	if !m.detailing {
		t.Error("clicking the selected row again should open detail")
	}
}

func TestClickPastLastRowDoesNothing(t *testing.T) {
	m := clickModel(t, 2)
	before := m.list.Cursor()
	m.Update(click(2, m.listTop()+10))
	if m.list.Cursor() != before || m.detailing {
		t.Error("click below the last row must be ignored")
	}
}

func TestClickOnPreviewPaneDoesNotSelect(t *testing.T) {
	m := clickModel(t, 3)
	if !m.ctx.SidebarOpen || m.ctx.PreviewWidth == 0 {
		t.Skip("preview not open in default test layout")
	}
	before := m.list.Cursor()
	m.Update(click(m.ctx.MainWidth+2, m.listTop()+2))
	if m.list.Cursor() != before {
		t.Error("click inside the preview pane must not move the list cursor")
	}
}

func TestClickIgnoredWhileModalOpen(t *testing.T) {
	m := clickModel(t, 3)
	m.ctrl.OpenText(action.ModeEdit, "JCT-1", "")
	before := m.list.Cursor()
	m.Update(click(2, m.listTop()+2))
	if m.list.Cursor() != before {
		t.Error("click under an open modal must be ignored")
	}
}

func TestClickPillSwitchesDetailTab(t *testing.T) {
	m := clickModel(t, 1)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open detail
	if !m.detailing {
		t.Fatal("detail did not open")
	}
	_ = m.View()
	labels := m.detailPillLabels()
	firstW := lipgloss.Width(m.ctx.Styles.TabActive.Render(labels[0]))
	m.Update(click(firstW+2, core.TopChromeRows+m.headerRows)) // inside second pill
	if m.detailTab != detailComments {
		t.Errorf("detailTab = %d, want comments after pill click", m.detailTab)
	}
}

func TestLensAtMapsChipCells(t *testing.T) {
	// chips render "[A] B C" style cells of len(name)+2 joined by one space.
	names := Lenses()
	if i, ok := lensAt(names, 0); !ok || i != 0 {
		t.Errorf("lensAt(0) = %d,%v want first lens", i, ok)
	}
	// One past the first cell is the separator space → still first cell? No:
	// separator is dead space between cells.
	firstW := len(names[0].Name) + 2
	if _, ok := lensAt(names, firstW); ok {
		t.Error("separator column should not hit a lens")
	}
	if i, ok := lensAt(names, firstW+1); !ok || i != 1 {
		t.Errorf("lensAt(second cell) = %d,%v want second lens", i, ok)
	}
}

func TestClickChipSwitchesLens(t *testing.T) {
	m := clickModel(t, 1)
	firstW := len(Lenses()[0].Name) + 2
	_, cmd := m.Update(click(firstW+1, core.TopChromeRows))
	if m.lens != 1 {
		t.Errorf("lens after chip click = %d, want 1", m.lens)
	}
	if cmd == nil {
		t.Error("switching lens by click should refetch")
	}
}

func TestSearchClickOnJQLBoxStartsEditing(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	_ = s.View()
	s.Update(click(4, core.TopChromeRows+1))
	if !s.editing {
		t.Error("click on the JQL box should start editing")
	}
}
