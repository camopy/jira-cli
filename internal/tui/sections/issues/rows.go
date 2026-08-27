// List-row rendering: the fixed-column layout plan, the
// per-cell renderers, and the local filter and lens chips above the list.

package issues

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	termansi "github.com/gechr/x/ansi"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pill"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/icons"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// priorityGlyphs maps Jira priority names to a single-rune arrow, Jira-style:
// up for higher-than-normal, down for lower. Unknown/empty priority renders a
// space so the column stays aligned. The glyphs come from the active icon set
// (tui.icons), read per call so a config reload reskins the next frame.
func priorityGlyph(name string) (string, bool) {
	ic := icons.Active()
	switch name {
	case "Highest":
		return ic.PriorityHighest, true
	case "High":
		return ic.PriorityHigh, true
	case "Medium":
		return ic.PriorityMedium, true
	case "Low":
		return ic.PriorityLow, true
	case "Lowest":
		return ic.PriorityLowest, true
	default:
		return "", false
	}
}

// typeGlyphFor maps an issue type name to a colored single-cell badge from
// the active icon set. Color is the primary signal; the shape is a secondary
// cue. Unknown types get the neutral glyph, empty a blank so the column
// stays aligned.
func typeGlyphFor(name string) string {
	ic := icons.Active()
	switch strings.ToLower(name) {
	case "epic":
		return theme.TypeEpic.Render(ic.Epic)
	case "story":
		return theme.TypeStory.Render(ic.Story)
	case "task":
		return theme.TypeTask.Render(ic.Task)
	case "sub-task", "subtask":
		return theme.TypeSubtask.Render(ic.Subtask)
	case "bug":
		return theme.TypeBug.Render(ic.Bug)
	case "":
		return " "
	default:
		return theme.TypeOther.Render(ic.UnknownType)
	}
}

// typeCell renders the issue's type badge for the list.
func typeCell(i *jira.Issue) string { return typeGlyphFor(issueTypeName(i)) }

// priorityCell renders the priority as a single colored glyph for the list.
func priorityCell(i *jira.Issue) string {
	p := issuePriority(i)
	g, ok := priorityGlyph(p)
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
	// below it trailing columns degrade so the summary stays readable.
	minSummary = 8
)

// rowLayout is the per-width column plan shared by rowText and columnHeader:
// which trailing columns fit and how wide the summary is. Columns degrade in
// order: assignee, age, then configured fields from last to first.
type rowLayout struct {
	assignee bool
	age      bool
	custom   []core.CustomField
	sumW     int
}

func layoutFor(width int, fields ...core.CustomField) rowLayout {
	custom := make([]core.CustomField, 0, len(fields))
	for _, field := range fields {
		if field.Column {
			custom = append(custom, field)
		}
	}
	layout := rowLayout{assignee: true, age: true, custom: custom}
	if layout.fit(width) {
		return layout
	}
	layout.assignee = false
	if layout.fit(width) {
		return layout
	}
	layout.age = false
	if layout.fit(width) {
		return layout
	}
	for len(layout.custom) > 0 {
		layout.custom = layout.custom[:len(layout.custom)-1]
		if layout.fit(width) {
			return layout
		}
	}
	layout.sumW = width - rowFixed
	return layout
}

func (l *rowLayout) fit(width int) bool {
	trailing := 0
	if l.assignee {
		trailing += assigneeCol + 2
	}
	if l.age {
		trailing += ageCol + 2
	}
	for _, field := range l.custom {
		trailing += customColumnWidth(field) + 2
	}
	l.sumW = width - rowFixed - trailing
	return l.sumW >= minSummary
}

func customColumnWidth(field core.CustomField) int {
	return min(max(lipgloss.Width(customFieldColumnLabel(field)), 6), 14)
}

// rowText renders one list row: a priority arrow, a fixed-width key, a colored
// status cell, the summary, then right-aligned assignee and relative-age
// columns. width is the total row budget; statusW is the view's widest status
// (see widestStatus) so the pills form an even column; trailing columns
// degrade per layoutFor when it is too narrow. The colored cells carry ANSI
// styling but a fixed display width, so listviewport (which measures display
// width) keeps the columns aligned.
func rowText(i *jira.Issue, width, statusW int, now time.Time, fields ...core.CustomField) string {
	key := fmt.Sprintf("%-*s", keyCol, xstrings.Truncate(issueKey(i), keyCol, "…"))
	left := typeCell(i) + " " + priorityCell(i) + " " + key + "  " + statusCell(i, statusW) + "  "
	l := layoutFor(width, fields...)
	if !l.age && !l.assignee && len(l.custom) == 0 {
		return termansi.Truncate(left+theme.CodeSpans(issueSummary(i)), max(width, 0), "")
	}
	// Truncate on the raw text, style after: CodeSpans keeps backticks, so
	// the styled cell is exactly as wide as the budgeted one.
	row := left + padRight(theme.CodeSpans(truncCells(issueSummary(i), l.sumW)), l.sumW)
	for _, field := range l.custom {
		value := customFieldValue(i, field)
		row += "  " + padRight(truncCells(value, customColumnWidth(field)), customColumnWidth(field))
	}
	if l.assignee {
		row += "  " + assigneeCell(i)
	}
	if l.age {
		row += "  " + fmt.Sprintf("%*s", ageCol, age(issueUpdated(i), now))
	}
	return row
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

// widestStatus measures the view's widest status name in cells, capped at
// the column budget — the shared pill width every row pads to, so the badges
// form an even column (the CLI's plain list applies the same normalization).
func widestStatus(issues []*jira.Issue) int {
	w := 0
	for _, iss := range issues {
		if n := lipgloss.Width(issueStatus(iss)); n > w {
			w = n
		}
	}
	return min(w, statusCol-2)
}

// statusCell renders the status as a filled pill padded to the column width —
// the same category-keyed fixed-color badge the CLI's plain issue list draws,
// so status reads identically everywhere. The name pads to statusW inside the
// fill, so every pill in the view is the same width; the padding beyond the
// pill stays unstyled so the row background shows through.
func statusCell(i *jira.Issue, statusW int) string {
	status := issueStatus(i)
	// truncCells, not rune truncation: status names are tenant text and a
	// wide-glyph name would otherwise overflow the column and shift the row.
	name := padRight(truncCells(status, statusCol-2), statusW)
	category, colorName := issueStatusCategory(i)
	badge := pill.Style(status, category, colorName).Render(" " + name + " ")
	return padRight(badge, statusCol)
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
func columnHeader(width int, fields ...core.CustomField) string {
	h := fmt.Sprintf("  %s %s %-*s  %-*s  ", "T", "!", keyCol, "KEY", statusCol, "STATUS")
	l := layoutFor(width, fields...)
	if !l.age && !l.assignee && len(l.custom) == 0 {
		h += "SUMMARY"
	} else {
		h += padRight("SUMMARY", l.sumW)
		for _, field := range l.custom {
			w := customColumnWidth(field)
			h += "  " + padRight(truncCells(strings.ToUpper(customFieldColumnLabel(field)), w), w)
		}
		if l.assignee {
			h += "  " + padRight("ASSIGNEE", assigneeCol)
		}
		if l.age {
			h += "  " + fmt.Sprintf("%*s", ageCol, "AGE")
		}
	}
	return termansi.Truncate(lipgloss.NewStyle().Faint(true).Render(h), max(width+2, 0), "")
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
