package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// detailTab identifies which tab is active in the detail view.
type detailTab int

const (
	tabSummary detailTab = iota
	tabComments
	tabWorklogs
	tabLinks
	tabCount
)

// issueDetail renders a full Jira issue across multiple tabs (summary,
// comments, worklogs, linked/subtasks). Each tab owns a viewport so
// scrolling is per-tab and content is cached. Mirrors pdc detail view.
type issueDetail struct {
	issue     jira.Issue
	width     int
	height    int
	activeTab detailTab
	viewports [tabCount]viewport.Model
}

func newIssueDetail(issue jira.Issue) issueDetail {
	var vps [tabCount]viewport.Model
	for i := range vps {
		vps[i] = viewport.New()
		vps[i].SoftWrap = true
	}
	d := issueDetail{issue: issue, viewports: vps}
	d.syncContent()
	return d
}

func (m issueDetail) Init() tea.Cmd { return nil }

func (m *issueDetail) setSize(width, height int) {
	m.width = width
	m.height = height
	for i := range m.viewports {
		m.viewports[i].SetWidth(width)
		m.viewports[i].SetHeight(max(height-1, 1)) // -1 for the tab bar
	}
}

func (m issueDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
		m.syncContent()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, nil
		case "shift+tab", "left":
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewports[m.activeTab], cmd = m.viewports[m.activeTab].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m issueDetail) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}
	tabs := m.tabBar()
	return tea.NewView(tabs + "\n" + m.viewports[m.activeTab].View())
}

// HeaderContent returns header chrome (tab bar + issue key) — App composes
// this above the body region, identical to pdc's incidentDetail.headerContent().
func (m issueDetail) HeaderContent() string {
	if m.issue.Key == nil {
		return ""
	}
	return theme.Title.Render(*m.issue.Key)
}

func (m *issueDetail) syncContent() {
	m.viewports[tabSummary].SetContent(m.summarySection())
	m.viewports[tabComments].SetContent(m.commentsSection())
	m.viewports[tabWorklogs].SetContent(m.worklogsSection())
	m.viewports[tabLinks].SetContent(m.linksSection())
}

func (m issueDetail) tabBar() string {
	tabs := []struct {
		name  string
		count int
	}{
		{"Summary", 0},
		{"Comments", len(m.issue.Comments)},
		{"Worklogs", len(m.issue.Worklogs)},
		{"Links", len(m.issue.LinkedIssues) + len(m.issue.Subtasks)},
	}
	active := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTitleFg).Underline(true).Padding(0, 1)
	inactive := lipgloss.NewStyle().Foreground(theme.ColorHeaderFg).Faint(true).Padding(0, 1)

	parts := make([]string, 0, len(tabs))
	for i, t := range tabs {
		label := t.name
		if t.count > 0 {
			label += fmt.Sprintf(" (%d)", t.count)
		}
		if detailTab(i) == m.activeTab {
			parts = append(parts, active.Render(label))
		} else {
			parts = append(parts, inactive.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func (m issueDetail) summarySection() string {
	const indent = "  "
	const labelW = 13
	lbl := func(name string) string {
		return theme.DetailLabel.Render(fmt.Sprintf("%*s:", labelW, name))
	}
	field := func(name, value string) string {
		return indent + lbl(name) + " " + theme.DetailValue.Render(value) + "\n"
	}
	var sb strings.Builder

	if m.issue.Key != nil {
		sb.WriteString(theme.DetailHeader.Render(*m.issue.Key))
		sb.WriteString("\n\n")
	}
	if f := m.issue.Fields; f != nil {
		if f.Summary != nil {
			sb.WriteString(field("Title", *f.Summary))
		}
		if f.Status != nil && f.Status.Name != nil {
			sb.WriteString(indent + lbl("Status") + " " + theme.StatusStyle(*f.Status.Name).Render(*f.Status.Name) + "\n")
		}
		if f.Priority != nil && f.Priority.Name != nil {
			styled := *f.Priority.Name
			if s, ok := theme.PriorityStyle(*f.Priority.Name); ok {
				styled = s.Render(*f.Priority.Name)
			}
			sb.WriteString(indent + lbl("Priority") + " " + styled + "\n")
		}
		if name := userDisplay(f.Assignee); name != "" {
			sb.WriteString(field("Assignee", name))
		}
		if name := userDisplay(f.Reporter); name != "" {
			sb.WriteString(field("Reporter", name))
		}
		if len(f.Labels) > 0 {
			sb.WriteString(field("Labels", strings.Join(f.Labels, ", ")))
		}
		if len(f.Components) > 0 {
			names := make([]string, 0, len(f.Components))
			for _, c := range f.Components {
				if c.Name != nil {
					names = append(names, *c.Name)
				}
			}
			sb.WriteString(field("Components", strings.Join(names, ", ")))
		}
		if f.Updated != nil {
			sb.WriteString(field("Updated", *f.Updated))
		}
		if f.Description != nil {
			sb.WriteString("\n")
			sb.WriteString(theme.DetailHeader.Render("Description"))
			sb.WriteString("\n\n")
			sb.WriteString(renderADFStyled(*f.Description, m.width-4, true))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (m issueDetail) commentsSection() string {
	if len(m.issue.Comments) == 0 {
		return theme.DetailDim.Render("  No comments\n")
	}
	var sb strings.Builder
	sb.WriteString(theme.DetailHeader.Render("Comments"))
	sb.WriteString("\n\n")
	for i, c := range m.issue.Comments {
		if c == nil {
			continue
		}
		author := userDisplay(c.Author)
		if author == "" {
			author = "anonymous"
		}
		sb.WriteString("  " + theme.EntityColor(author).Render(author) + "\n")
		if c.Body != nil {
			sb.WriteString(renderADFStyled(*c.Body, m.width-4, true))
			sb.WriteString("\n")
		}
		if i < len(m.issue.Comments)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (m issueDetail) worklogsSection() string {
	if len(m.issue.Worklogs) == 0 {
		return theme.DetailDim.Render("  No worklogs\n")
	}
	var sb strings.Builder
	sb.WriteString(theme.DetailHeader.Render("Worklogs"))
	sb.WriteString("\n\n")
	for _, w := range m.issue.Worklogs {
		if w == nil {
			continue
		}
		secs := 0
		if w.TimeSpentSeconds != nil {
			secs = *w.TimeSpentSeconds
		}
		when := ""
		if w.Started != nil {
			when = *w.Started
		}
		fmt.Fprintf(&sb, "  %s  %s\n",
			theme.HelpKey.Render(formatDuration(secs)),
			theme.DetailDim.Render(when))
		if w.Comment != nil {
			sb.WriteString(renderADFStyled(*w.Comment, m.width-4, true))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (m issueDetail) linksSection() string {
	var sb strings.Builder
	if len(m.issue.LinkedIssues) > 0 {
		sb.WriteString(theme.DetailHeader.Render("Linked"))
		sb.WriteString("\n\n")
		for _, l := range m.issue.LinkedIssues {
			if l != nil && l.Key != nil {
				sb.WriteString("  " + theme.HelpKey.Render(*l.Key))
				if l.Fields != nil && l.Fields.Summary != nil {
					sb.WriteString("  " + *l.Fields.Summary)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	if len(m.issue.Subtasks) > 0 {
		sb.WriteString(theme.DetailHeader.Render("Subtasks"))
		sb.WriteString("\n\n")
		for _, s := range m.issue.Subtasks {
			if s != nil && s.Key != nil {
				sb.WriteString("  " + theme.HelpKey.Render(*s.Key))
				if s.Fields != nil && s.Fields.Summary != nil {
					sb.WriteString("  " + *s.Fields.Summary)
				}
				sb.WriteString("\n")
			}
		}
	}
	if sb.Len() == 0 {
		return theme.DetailDim.Render("  No links or subtasks\n")
	}
	return sb.String()
}

func userDisplay(u *jira.User) string {
	if u == nil {
		return ""
	}
	if u.DisplayName != nil {
		return *u.DisplayName
	}
	if u.EmailAddress != nil {
		return *u.EmailAddress
	}
	if u.AccountID != nil {
		return *u.AccountID
	}
	return ""
}

// formatDuration renders seconds as a Jira-style worklog string.
func formatDuration(secs int) string {
	if secs <= 0 {
		return "0s"
	}
	d := secs / 86400
	h := (secs % 86400) / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	parts := []string{}
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%dd", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if len(parts) == 0 && s > 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	return strings.Join(parts, " ")
}
