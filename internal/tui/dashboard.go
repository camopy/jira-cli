package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/jira"
)

// Dashboard wraps the issues list view and forwards the body region's
// WindowSizeMsg to it. Mirrors pdc/internal/tui/dashboard.go: the wrapper
// owns no chrome — header, status bar and overlays live at App level.
type Dashboard struct {
	issues issuesList
	width  int
	height int
}

func newDashboard(ctx context.Context, baseURL string) Dashboard {
	return Dashboard{issues: newIssuesList(ctx, baseURL)}
}

func (d Dashboard) Init() tea.Cmd { return nil }

// SetIssues updates the list with fresh data.
func (d *Dashboard) SetIssues(issues []*jira.Issue) {
	d.issues.SetIssues(issues)
}

// FilterActive reports whether the list filter input is focused.
func (d Dashboard) FilterActive() bool {
	return d.issues.FilterActive()
}

// SelectedIssue returns the currently highlighted issue, if any.
func (d Dashboard) SelectedIssue() *jira.Issue {
	return d.issues.SelectedIssue()
}

func (d Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		d.width = ws.Width
		d.height = ws.Height
		d.issues.width = ws.Width
		d.issues.height = ws.Height
	}
	im, cmd := d.issues.Update(msg)
	d.issues = im.(issuesList)
	return d, cmd
}

func (d Dashboard) View() tea.View {
	if d.width == 0 {
		return tea.NewView("")
	}
	return tea.NewView(lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		MaxHeight(d.height).
		Render(d.issues.View().Content))
}
