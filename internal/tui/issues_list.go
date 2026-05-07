package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/components"
	"github.com/matcra587/jira-cli/internal/tui/theme"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// issuesList is the dashboard's issue list view: a real Bubble Tea model
// with a viewport, themed columns, cursor highlighting and an inline
// filter input. Mirrors pdc/internal/tui/incidents_list.go.
type issuesList struct {
	ctx          context.Context //nolint:containedctx // models are value-typed; ctx travels with them
	baseURL      string
	issues       []*jira.Issue
	searchIndex  []string
	cursor       int
	scrollOffset int
	width        int
	height       int
	filterInput  textinput.Model
	filterActive bool
	keys         listKeyMap

	// Structured filter (set via SetFilterState; applied alongside the
	// text filterInput in `visible`).
	filterState components.IssueFilterState
	myEmail     string
	myAccount   string
	teamIDs     []string
}

func newIssuesList(ctx context.Context, baseURL string) issuesList {
	fi := textinput.New()
	fi.Prompt = ""
	fi.CharLimit = 80
	return issuesList{
		ctx:         ctx,
		baseURL:     baseURL,
		filterInput: fi,
		keys:        newListKeyMap(),
		filterState: components.DefaultIssueFilterState(),
	}
}

// SetFilterState applies a structured filter and the identity used to
// resolve "me" / "team" choices. Cursor is clamped against the new visible
// set so the highlighted row stays in range.
func (m *issuesList) SetFilterState(state components.IssueFilterState, email, accountID string, teamIDs []string) {
	m.filterState = state
	m.myEmail = email
	m.myAccount = accountID
	m.teamIDs = append(m.teamIDs[:0], teamIDs...)
	vis := m.visible()
	if m.cursor >= len(vis) {
		m.cursor = max(0, len(vis)-1)
	}
	if m.scrollOffset > m.cursor {
		m.scrollOffset = m.cursor
	}
}

func (m issuesList) Init() tea.Cmd { return nil }

// SetIssues replaces the issue list and rebuilds the search index.
func (m *issuesList) SetIssues(issues []*jira.Issue) {
	m.issues = issues
	m.searchIndex = make([]string, len(issues))
	for i, iss := range issues {
		m.searchIndex[i] = buildIssueSearchEntry(iss)
	}
	vis := m.visible()
	if m.cursor >= len(vis) {
		m.cursor = max(0, len(vis)-1)
	}
	if m.scrollOffset > m.cursor {
		m.scrollOffset = m.cursor
	}
}

// FilterActive reports whether the filter input is focused.
func (m issuesList) FilterActive() bool { return m.filterActive }

// SelectedIssue returns the currently highlighted issue, if any.
func (m issuesList) SelectedIssue() *jira.Issue {
	vis := m.visible()
	if len(vis) == 0 {
		return nil
	}
	if m.cursor < 0 || m.cursor >= len(vis) {
		return vis[0]
	}
	return vis[m.cursor]
}

// FilterValue exposes the current filter for status-bar use.
func (m issuesList) FilterValue() string { return m.filterInput.Value() }

func (m issuesList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.filterActive {
		return m.updateFilterInput(k)
	}
	return m.updateNormalKey(k)
}

func (m issuesList) updateFilterInput(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.filterActive = false
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil
	case "enter":
		m.filterInput.Blur()
		m.filterActive = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(k)
		vis := m.visible()
		if m.cursor >= len(vis) {
			m.cursor = max(0, len(vis)-1)
		}
		if m.scrollOffset > m.cursor {
			m.scrollOffset = m.cursor
		}
		return m, cmd
	}
}

func (m issuesList) updateNormalKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	vis := m.visible()
	switch {
	case key.Matches(k, m.keys.Filter):
		m.filterActive = true
		cmd := m.filterInput.Focus()
		return m, tea.Batch(cmd, textinput.Blink)
	case key.Matches(k, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(k, m.keys.Down):
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case key.Matches(k, m.keys.Top):
		m.cursor = 0
	case key.Matches(k, m.keys.Bottom):
		if len(vis) > 0 {
			m.cursor = len(vis) - 1
		}
	}
	rows := m.viewportRows()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+rows {
		m.scrollOffset = m.cursor - rows + 1
	}
	return m, nil
}

func (m issuesList) viewportRows() int {
	rows := m.height - 2 // header line + border
	if m.filterActive || m.filterInput.Value() != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m issuesList) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}
	var sb strings.Builder

	showFilter := m.filterActive || m.filterInput.Value() != ""
	if showFilter {
		if m.filterActive {
			sb.WriteString("  / " + m.filterInput.View() + "\n")
		} else {
			sb.WriteString("  filter: " + m.filterInput.Value() + "\n")
		}
	}

	vis := m.visible()
	if len(vis) == 0 {
		msg := "No issues"
		if showFilter {
			msg = "No matching issues"
		}
		body := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Faint(true).Render(msg))
		sb.WriteString(body)
		return tea.NewView(sb.String())
	}

	widths := layoutListColumns(m.width)
	sb.WriteString(m.renderHeader(widths) + "\n")

	rows := m.viewportRows()
	start := min(m.scrollOffset, max(0, len(vis)-rows))
	for i := start; i < len(vis) && (i-start) < rows; i++ {
		sb.WriteString(m.renderRow(vis[i], i == m.cursor, widths))
		sb.WriteString("\n")
	}
	return tea.NewView(sb.String())
}

// visible returns the issues that match BOTH the text filter and the
// structured filter state.
func (m issuesList) visible() []*jira.Issue {
	q := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	hasStructured := !m.filterState.IsDefault()

	if q == "" && !hasStructured {
		return m.issues
	}
	out := make([]*jira.Issue, 0, len(m.issues))
	for i, iss := range m.issues {
		if q != "" {
			if i < len(m.searchIndex) {
				if !strings.Contains(m.searchIndex[i], q) {
					continue
				}
			} else if !matchesIssueQuery(iss, q) {
				continue
			}
		}
		if hasStructured && !matchesStructured(iss, m.filterState, m.myEmail, m.myAccount, m.teamIDs) {
			continue
		}
		out = append(out, iss)
	}
	return out
}

// matchesStructured applies the IssueFilterState to a single issue. Each
// field defaults to "all" (no filtering); non-default values gate the row.
func matchesStructured(iss *jira.Issue, fs components.IssueFilterState, myEmail, myAccount string, teamIDs []string) bool {
	if iss == nil || iss.Fields == nil {
		return false
	}
	switch fs.Assignment {
	case "me":
		if !assigneeIsUser(iss.Fields.Assignee, myEmail, myAccount) {
			return false
		}
	case "unassigned":
		if iss.Fields.Assignee != nil {
			return false
		}
	case "team":
		if !assigneeInTeam(iss.Fields.Assignee, teamIDs) {
			return false
		}
	case "all", "":
		// no-op
	}
	if fs.Status != "" && fs.Status != "all" {
		got := ""
		if iss.Fields.Status != nil && iss.Fields.Status.Name != nil {
			got = strings.ToLower(*iss.Fields.Status.Name)
		}
		// Match the choice against either the literal status or its
		// category (open / in-progress / done).
		if !statusMatchesChoice(got, fs.Status) {
			return false
		}
	}
	if fs.Priority != "" && fs.Priority != "all" {
		got := ""
		if iss.Fields.Priority != nil && iss.Fields.Priority.Name != nil {
			got = strings.ToLower(*iss.Fields.Priority.Name)
		}
		if got != strings.ToLower(fs.Priority) {
			return false
		}
	}
	return true
}

func assigneeIsUser(u *jira.User, email, accountID string) bool {
	if u == nil {
		return false
	}
	if accountID != "" && u.AccountID != nil && *u.AccountID == accountID {
		return true
	}
	if email != "" && u.EmailAddress != nil && strings.EqualFold(*u.EmailAddress, email) {
		return true
	}
	return false
}

func assigneeInTeam(u *jira.User, teamIDs []string) bool {
	if u == nil || u.AccountID == nil {
		return false
	}
	for _, id := range teamIDs {
		if id == *u.AccountID {
			return true
		}
	}
	return false
}

// statusMatchesChoice maps a Jira status name to the open/in-progress/done
// category buckets so the filter matches a class of statuses, not a single
// literal name. "open" covers To Do / Backlog / Open / New.
func statusMatchesChoice(got, choice string) bool {
	got = strings.ToLower(got)
	choice = strings.ToLower(choice)
	if got == choice {
		return true
	}
	switch choice {
	case "open":
		switch got {
		case "to do", "todo", "open", "backlog", "new":
			return true
		}
	case "in-progress":
		switch got {
		case "in progress", "in review", "in-progress":
			return true
		}
	case "done":
		switch got {
		case "done", "closed", "resolved", "complete":
			return true
		}
	}
	return false
}

func buildIssueSearchEntry(iss *jira.Issue) string {
	var parts []string
	if iss == nil {
		return ""
	}
	if iss.Key != nil {
		parts = append(parts, strings.ToLower(*iss.Key))
	}
	if iss.Fields != nil {
		if iss.Fields.Summary != nil {
			parts = append(parts, strings.ToLower(*iss.Fields.Summary))
		}
		if iss.Fields.Status != nil && iss.Fields.Status.Name != nil {
			parts = append(parts, strings.ToLower(*iss.Fields.Status.Name))
		}
		if iss.Fields.Assignee != nil && iss.Fields.Assignee.DisplayName != nil {
			parts = append(parts, strings.ToLower(*iss.Fields.Assignee.DisplayName))
		}
		if iss.Fields.Priority != nil && iss.Fields.Priority.Name != nil {
			parts = append(parts, strings.ToLower(*iss.Fields.Priority.Name))
		}
	}
	// Null separator prevents cross-field substring matches (e.g. "ab"+"cd"
	// can't match query "bc"). Same trick as pagerduty-client.
	return strings.Join(parts, "\x00")
}

func matchesIssueQuery(iss *jira.Issue, q string) bool {
	if iss == nil {
		return false
	}
	if iss.Key != nil && strings.Contains(strings.ToLower(*iss.Key), q) {
		return true
	}
	if iss.Fields == nil {
		return false
	}
	if iss.Fields.Summary != nil && strings.Contains(strings.ToLower(*iss.Fields.Summary), q) {
		return true
	}
	if iss.Fields.Status != nil && iss.Fields.Status.Name != nil &&
		strings.Contains(strings.ToLower(*iss.Fields.Status.Name), q) {
		return true
	}
	return false
}

// columnWidths intentionally fixed for KEY/STATUS/PRIORITY/ASSIGNEE/UPDATED,
// SUMMARY flexes to the remainder.
type listColumn struct {
	header string
	width  int
	flex   bool
}

var listColumns = []listColumn{
	{"KEY", 12, false},
	{"STATUS", 14, false},
	{"PRIORITY", 9, false},
	{"SUMMARY", 0, true},
	{"ASSIGNEE", 18, false},
	{"UPDATED", 16, false},
}

func layoutListColumns(termWidth int) []int {
	widths := make([]int, len(listColumns))
	used := 0
	gaps := len(listColumns) - 1 // single space between cols
	for i, c := range listColumns {
		if !c.flex {
			widths[i] = c.width
			used += c.width
		}
	}
	used += gaps
	rem := termWidth - used
	if rem < 12 {
		rem = 12
	}
	for i, c := range listColumns {
		if c.flex {
			widths[i] = rem
		}
	}
	return widths
}

func (m issuesList) renderHeader(widths []int) string {
	parts := make([]string, 0, len(listColumns))
	for i, c := range listColumns {
		parts = append(parts, fmt.Sprintf("%-*s", widths[i], c.header))
	}
	return theme.TableHeader.Render(strings.Join(parts, " "))
}

func (m issuesList) renderRow(iss *jira.Issue, isCursor bool, widths []int) string {
	key := ""
	summary := ""
	status := ""
	priority := ""
	assignee := ""
	updated := ""
	if iss != nil {
		if iss.Key != nil {
			key = *iss.Key
		}
		if iss.Fields != nil {
			if iss.Fields.Summary != nil {
				summary = *iss.Fields.Summary
			}
			if iss.Fields.Status != nil && iss.Fields.Status.Name != nil {
				status = *iss.Fields.Status.Name
			}
			if iss.Fields.Priority != nil && iss.Fields.Priority.Name != nil {
				priority = *iss.Fields.Priority.Name
			}
			if iss.Fields.Assignee != nil && iss.Fields.Assignee.DisplayName != nil {
				assignee = *iss.Fields.Assignee.DisplayName
			}
			if iss.Fields.Updated != nil {
				updated = shortenTimestamp(*iss.Fields.Updated)
			}
		}
	}

	cells := []string{
		styleKey(key, widths[0]),
		theme.StatusStyle(status).Render(padRight(status, widths[1])),
		stylePriority(priority, widths[2]),
		padRight(truncate(summary, widths[3]), widths[3]),
		theme.EntityColor(assignee).Render(padRight(truncate(orDash(assignee), widths[4]), widths[4])),
		theme.DetailDim.Render(padRight(updated, widths[5])),
	}
	row := strings.Join(cells, " ")
	if isCursor {
		return components.PersistBgFull(row, theme.CursorBg, m.width)
	}
	return row
}

func styleKey(key string, w int) string {
	if key == "" {
		return strings.Repeat(" ", w)
	}
	return theme.HelpKey.Render(padRight(key, w))
}

func stylePriority(p string, w int) string {
	if style, ok := theme.PriorityStyle(p); ok {
		return style.Render(padRight(p, w))
	}
	return padRight(orDash(p), w)
}

func padRight(s string, w int) string {
	s = truncate(s, w)
	if l := lipgloss.Width(s); l < w {
		s += strings.Repeat(" ", w-l)
	}
	return s
}

func truncate(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	out := []rune{}
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > w-1 {
			break
		}
		out = append(out, r)
		width += rw
	}
	return string(out) + "…"
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// shortenTimestamp keeps just the date portion of an ISO-8601 string for
// the table column. Falls back to the input on parse failure.
func shortenTimestamp(s string) string {
	if i := strings.Index(s, "T"); i > 0 {
		date := s[:i]
		// Add HH:MM if there's room for it.
		if len(s) >= i+6 {
			return date + " " + s[i+1:i+6]
		}
		return date
	}
	return s
}
