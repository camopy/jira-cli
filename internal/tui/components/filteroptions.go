package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// IssueFilterState captures the structured filter applied to the issue list.
// Defaults are the "show everything" values, so a fresh state matches all
// issues (other than the text filter, which is independent).
type IssueFilterState struct {
	// Assignment ∈ {all, me, unassigned, team}.
	Assignment string
	// Status ∈ {all, open, in-progress, done}.
	Status string
	// Priority ∈ {all, highest, high, medium, low, lowest}.
	Priority string
}

// DefaultIssueFilterState returns the "match everything" defaults.
func DefaultIssueFilterState() IssueFilterState {
	return IssueFilterState{Assignment: "all", Status: "all", Priority: "all"}
}

// IsDefault reports whether the state matches DefaultIssueFilterState.
func (s IssueFilterState) IsDefault() bool {
	return s == DefaultIssueFilterState()
}

// ChipSummary returns a space-separated `key:value` summary of fields that
// differ from the defaults — used for the status bar hint and tests.
func (s IssueFilterState) ChipSummary() string {
	d := DefaultIssueFilterState()
	var chips []string
	if s.Assignment != d.Assignment {
		chips = append(chips, "assignment:"+s.Assignment)
	}
	if s.Status != d.Status {
		chips = append(chips, "status:"+s.Status)
	}
	if s.Priority != d.Priority {
		chips = append(chips, "priority:"+s.Priority)
	}
	return strings.Join(chips, " ")
}

// FilterAppliedMsg is emitted when the user confirms a filter selection.
type FilterAppliedMsg struct {
	Origin     string
	Selections map[string]string
}

// FilterClosed is emitted when the user dismisses the overlay.
type FilterClosed struct{}

// FilterRow describes a single row in the filter overlay.
type FilterRow struct {
	Label   string
	Choices []string
	Current int
}

// FilterOptions is a Bubble Tea overlay that lets the user cycle through
// per-field choices. Data-driven so callers supply rows at show-time.
type FilterOptions struct {
	Visible bool
	origin  string
	cursor  int
	rows    []FilterRow
}

// NewFilterOptions returns an empty overlay ready to be Show()n.
func NewFilterOptions() FilterOptions { return FilterOptions{} }

// IssueFilterRows returns the default rows for the issues tab.
func IssueFilterRows(state IssueFilterState) []FilterRow {
	return []FilterRow{
		{Label: "Assignment", Choices: []string{"all", "me", "unassigned", "team"}, Current: indexOf([]string{"all", "me", "unassigned", "team"}, state.Assignment, 0)},
		{Label: "Status", Choices: []string{"all", "open", "in-progress", "done"}, Current: indexOf([]string{"all", "open", "in-progress", "done"}, state.Status, 0)},
		{Label: "Priority", Choices: []string{"all", "highest", "high", "medium", "low", "lowest"}, Current: indexOf([]string{"all", "highest", "high", "medium", "low", "lowest"}, state.Priority, 0)},
	}
}

func indexOf(haystack []string, needle string, fallback int) int {
	for i, v := range haystack {
		if v == needle {
			return i
		}
	}
	return fallback
}

// Selections returns label → currently-selected-value for every row.
func (f FilterOptions) Selections() map[string]string {
	m := make(map[string]string, len(f.rows))
	for _, row := range f.rows {
		m[row.Label] = row.Choices[row.Current]
	}
	return m
}

// Origin returns the tab id that opened the overlay.
func (f FilterOptions) Origin() string { return f.origin }

// IssueState reads selections back into a typed IssueFilterState.
func (f FilterOptions) IssueState() IssueFilterState {
	sel := f.Selections()
	return IssueFilterState{
		Assignment: firstNonEmpty(sel["Assignment"], "all"),
		Status:     firstNonEmpty(sel["Status"], "all"),
		Priority:   firstNonEmpty(sel["Priority"], "all"),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ShowWithRows opens the overlay with the given rows and origin tag.
// Reassign: f = f.ShowWithRows("issues", IssueFilterRows(state)).
func (f FilterOptions) ShowWithRows(origin string, rows []FilterRow) FilterOptions {
	f.Visible = true
	f.origin = origin
	f.rows = rows
	f.cursor = 0
	return f
}

func (f FilterOptions) Init() tea.Cmd { return nil }

func (f FilterOptions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !f.Visible {
		return f, nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	switch k.String() {
	case "j", "down":
		if f.cursor < len(f.rows)-1 {
			f.cursor++
		}
	case "k", "up":
		if f.cursor > 0 {
			f.cursor--
		}
	case "space", "l", "right":
		row := &f.rows[f.cursor]
		row.Current = (row.Current + 1) % len(row.Choices)
	case "h", "left":
		row := &f.rows[f.cursor]
		row.Current = (row.Current - 1 + len(row.Choices)) % len(row.Choices)
	case "backspace":
		row := &f.rows[f.cursor]
		row.Current = 0
	case "enter":
		f.Visible = false
		origin := f.origin
		sel := f.Selections()
		return f, func() tea.Msg { return FilterAppliedMsg{Origin: origin, Selections: sel} }
	case "esc":
		f.Visible = false
		return f, func() tea.Msg { return FilterClosed{} }
	}
	return f, nil
}

func (f FilterOptions) View() tea.View {
	if !f.Visible {
		return tea.NewView("")
	}
	var sb strings.Builder
	sb.WriteString(theme.Title.Render("Filter Options"))
	sb.WriteString("\n\n")

	maxLabel := 0
	for _, row := range f.rows {
		if w := len(row.Label) + 1; w > maxLabel {
			maxLabel = w
		}
	}

	dim := theme.HelpDesc
	for i, row := range f.rows {
		cursor := "  "
		if i == f.cursor {
			cursor = "❯ "
		}
		label := dim.Render(fmt.Sprintf("%-*s", maxLabel, row.Label+":"))
		choices := make([]string, 0, len(row.Choices))
		for j, c := range row.Choices {
			if j == row.Current {
				choices = append(choices, theme.HelpKey.Render(c))
			} else {
				choices = append(choices, dim.Render(c))
			}
		}
		sb.WriteString(cursor + label + "  " + strings.Join(choices, dim.Render(" | ")) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dim.Render("↑↓ navigate  ←→ select  enter apply  esc close  ⌫ reset"))

	return tea.NewView(RenderOverlay(sb.String(), 52))
}
