// List-row rendering: the fixed-column layout plan, the
// per-cell renderers, and the local filter and lens chips above the list.

package issues

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pill"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// priorityGlyphs maps Jira priority names to a single-rune arrow, Jira-style:
// up for higher-than-normal, down for lower. Unknown/empty priority renders a
// space so the column stays aligned.
var priorityGlyphs = map[string]string{
	"Highest": "↟",
	"High":    "↑",
	"Medium":  "=",
	"Low":     "↓",
	"Lowest":  "↡",
}

// typeGlyphFor maps an issue type name to a colored single-rune badge. Color is
// the primary signal; the shape is a secondary cue. Unknown types get a neutral
// diamond, empty a blank so the column stays aligned.
func typeGlyphFor(name string) string {
	switch strings.ToLower(name) {
	case "epic":
		return lipgloss.NewStyle().Foreground(theme.Theme.Magenta.GetForeground()).Render("◆")
	case "story":
		return lipgloss.NewStyle().Foreground(theme.Theme.Green.GetForeground()).Render("●")
	case "task":
		return lipgloss.NewStyle().Foreground(theme.Theme.Blue.GetForeground()).Render("■")
	case "sub-task", "subtask":
		return lipgloss.NewStyle().Foreground(theme.Theme.Blue.GetForeground()).Faint(true).Render("▸")
	case "bug":
		return lipgloss.NewStyle().Foreground(theme.Theme.Red.GetForeground()).Render("▲")
	case "":
		return " "
	default:
		return lipgloss.NewStyle().Foreground(theme.Theme.Yellow.GetForeground()).Render("◇")
	}
}

// typeCell renders the issue's type badge for the list.
func typeCell(i *jira.Issue) string { return typeGlyphFor(issueTypeName(i)) }

// priorityCell renders the priority as a single colored arrow for the list.
func priorityCell(i *jira.Issue) string {
	p := issuePriority(i)
	g, ok := priorityGlyphs[p]
	if !ok {
		return " "
	}
	if st, ok := theme.PriorityStyle(p); ok {
		return st.Render(g)
	}
	return g
}

// keyCol/statusCol/assigneeCol/ageCol are the fixed column widths for the
// list; the summary takes the remaining width. rowFixed is the display width
// everything before the summary occupies (type 2 + priority 2 + key + 2 +
// status + 2).
const (
	keyCol      = 12
	statusCol   = 13
	assigneeCol = 10
	ageCol      = 4
	rowFixed    = 4 + keyCol + 2 + statusCol + 2
	// minSummary is the narrowest summary worth keeping trailing columns for;
	// below it they degrade (assignee first, then age) so the summary stays
	// readable.
	minSummary = 8
)

// rowLayout is the per-width column plan shared by rowText and columnHeader:
// which trailing columns fit and how wide the summary is. Columns degrade in
// order — assignee first, then age — as the terminal narrows.
type rowLayout struct {
	assignee bool
	age      bool
	sumW     int
}

func layoutFor(width int) rowLayout {
	if sumW := width - rowFixed - assigneeCol - 2 - ageCol - 2; sumW >= minSummary {
		return rowLayout{assignee: true, age: true, sumW: sumW}
	}
	if sumW := width - rowFixed - ageCol - 2; sumW >= minSummary {
		return rowLayout{age: true, sumW: sumW}
	}
	return rowLayout{sumW: width - rowFixed}
}

// rowText renders one list row: a priority arrow, a fixed-width key, a colored
// status cell, the summary, then right-aligned assignee and relative-age
// columns. width is the total row budget; trailing columns
// degrade per layoutFor when it is too narrow. The colored cells carry ANSI
// styling but a fixed display width, so listviewport (which measures display
// width) keeps the columns aligned.
func rowText(i *jira.Issue, width int, now time.Time) string {
	key := fmt.Sprintf("%-*s", keyCol, xstrings.Truncate(issueKey(i), keyCol, "…"))
	left := typeCell(i) + " " + priorityCell(i) + " " + key + "  " + statusCell(i) + "  "
	l := layoutFor(width)
	if !l.age {
		return left + theme.CodeSpans(issueSummary(i))
	}
	// Truncate on the raw text, style after: CodeSpans keeps backticks, so
	// the styled cell is exactly as wide as the budgeted one.
	row := left + padRight(theme.CodeSpans(truncCells(issueSummary(i), l.sumW)), l.sumW)
	if l.assignee {
		row += "  " + assigneeCell(i)
	}
	return row + "  " + fmt.Sprintf("%*s", ageCol, age(issueUpdated(i), now))
}

// assigneeCell renders the assignee in a fixed-width column, colored by name.
// Unassigned issues show a dim dash.
func assigneeCell(i *jira.Issue) string {
	name := issueAssignee(i)
	if name == "Unassigned" {
		return theme.DetailDim.Render(padRight("—", assigneeCol))
	}
	return theme.EntityColor(name).Render(padRight(truncCells(name, assigneeCol), assigneeCol))
}

// statusCell renders the status as a filled pill padded to the column width —
// the same category-keyed fixed-color badge the CLI's plain issue list draws,
// so status reads identically everywhere. The pill hugs the name; the padding
// stays unstyled so the row background shows through.
func statusCell(i *jira.Issue) string {
	status := issueStatus(i)
	// truncCells, not rune truncation: status names are tenant text and a
	// wide-glyph name would otherwise overflow the column and shift the row.
	name := truncCells(status, statusCol-2)
	category, colorName := issueStatusCategory(i)
	badge := pill.Style(status, category, colorName).Render(" " + name + " ")
	pad := statusCol - lipgloss.Width(badge)
	if pad < 0 {
		pad = 0
	}
	return badge + strings.Repeat(" ", pad)
}

// statusPill renders the status as a filled pill for detail headers, from the
// same shared palette as the list rows and the CLI.
func statusPill(i *jira.Issue) string {
	status := issueStatus(i)
	if status == "" {
		return ""
	}
	category, colorName := issueStatusCategory(i)
	return pill.Style(status, category, colorName).Render(" " + status + " ")
}

// columnHeader is the dim heading row above the list, derived from the same
// layoutFor plan as the rows so it can never drift out of alignment. The two
// leading spaces align it under the selection-marker column the rows carry.
func columnHeader(width int) string {
	h := fmt.Sprintf("  %s %s %-*s  %-*s  ", "T", "!", keyCol, "KEY", statusCol, "STATUS")
	l := layoutFor(width)
	if !l.age {
		h += "SUMMARY"
	} else {
		h += padRight("SUMMARY", l.sumW)
		if l.assignee {
			h += "  " + padRight("ASSIGNEE", assigneeCol)
		}
		h += "  " + fmt.Sprintf("%*s", ageCol, "AGE")
	}
	return lipgloss.NewStyle().Faint(true).Render(h)
}

// padRight pads s with spaces to w display cells. fmt's %-*s pads by bytes,
// which misaligns multi-byte summaries; this measures terminal cells so wide
// glyphs (CJK, emoji) keep the age column aligned.
func padRight(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// truncCells shortens s to at most w display cells, adding an ellipsis. Unlike
// truncate (which counts runes), it measures cells so a 2-cell CJK glyph can't
// blow the column budget.
func truncCells(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var (
		out  strings.Builder
		used int
	)
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 { // reserve one cell for the ellipsis
			break
		}
		out.WriteRune(r)
		used += rw
	}
	return out.String() + "…"
}

// matchesFilter reports whether an issue matches a local filter query via a
// case-insensitive substring over the fields a user would search by.
func matchesFilter(i *jira.Issue, query string) bool {
	if query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{
		issueKey(i), issueSummary(i), issueStatus(i), issueAssignee(i),
	}, " "))
	return strings.Contains(hay, strings.ToLower(query))
}

// filterIssues returns the subset matching the query, preserving order.
func filterIssues(issues []*jira.Issue, query string) []*jira.Issue {
	if query == "" {
		return issues
	}
	out := make([]*jira.Issue, 0, len(issues))
	for _, iss := range issues {
		if matchesFilter(iss, query) {
			out = append(out, iss)
		}
	}
	return out
}

// chips renders the lens quick-filters with the active one bracketed.
func chips(lenses []Lens, active int) string {
	parts := make([]string, len(lenses))
	for idx, l := range lenses {
		if idx == active {
			parts[idx] = "[" + l.Name + "]"
		} else {
			parts[idx] = " " + l.Name + " "
		}
	}
	return strings.Join(parts, " ")
}

// lensAt maps a click x on the chips row to a lens index by walking the same
// cell widths chips() renders ("[Name]" or " Name ", joined by one space).
func lensAt(lenses []Lens, x int) (int, bool) {
	pos := 0
	for i, l := range lenses {
		w := lipgloss.Width(l.Name) + 2 // brackets or pad spaces — same width either way
		if x >= pos && x < pos+w {
			return i, true
		}
		pos += w + 1
	}
	return 0, false
}

// chipsWithQuery renders the lens chips with the active lens's JQL appended
// faint and truncated to the remaining width,
// kept to one row. Too-narrow terminals just get the chips.
func chipsWithQuery(lenses []Lens, active, width int) string {
	c := chips(lenses, active)
	room := width - lipgloss.Width(c) - 4
	if room < 12 {
		return c
	}
	return c + "    " + lipgloss.NewStyle().Faint(true).Render(xstrings.Truncate(lenses[active].JQL, room, "…"))
}
