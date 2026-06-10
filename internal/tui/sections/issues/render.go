package issues

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/markdown"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// fetchFields are the issue fields the list+sidebar need; requesting a narrow
// set keeps the response small. "description" feeds the sidebar detail pane.
var fetchFields = []string{"summary", "issuetype", "status", "assignee", "reporter", "priority", "labels", "updated", "description"}

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

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func issueKey(i *jira.Issue) string { return deref(i.Key) }

func issueSummary(i *jira.Issue) string {
	if i.Fields == nil {
		return ""
	}
	return deref(i.Fields.Summary)
}

func issueStatus(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Status == nil {
		return ""
	}
	return deref(i.Fields.Status.Name)
}

func issueAssignee(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Assignee == nil {
		return "Unassigned"
	}
	if name := deref(i.Fields.Assignee.DisplayName); name != "" {
		return name
	}
	return "Unassigned"
}

func issuePriority(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Priority == nil {
		return ""
	}
	return deref(i.Fields.Priority.Name)
}

func issueReporter(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Reporter == nil {
		return ""
	}
	return deref(i.Fields.Reporter.DisplayName)
}

func issueTypeName(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.IssueType == nil {
		return ""
	}
	return deref(i.Fields.IssueType.Name)
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

func issueUpdated(i *jira.Issue) string {
	if i.Fields == nil {
		return ""
	}
	return deref(i.Fields.Updated)
}

// projectOf returns the project prefix of an issue key ("JCT-12" → "JCT").
func projectOf(key string) string {
	if i := strings.IndexByte(key, '-'); i > 0 {
		return key[:i]
	}
	return ""
}

// jiraTimeLayout is the timestamp format Jira's REST API returns. Some
// deployments emit a colon in the zone offset (+05:30) instead, which RFC3339
// covers — age falls back to it.
const jiraTimeLayout = "2006-01-02T15:04:05.000-0700"

// age renders a relative age for a Jira timestamp (30s, 5m, 2h, 4d, 3w, 6mo,
// 2y). Empty or unparsable timestamps render "".
func age(ts string, now time.Time) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(jiraTimeLayout, ts)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, ts); err != nil {
			return ""
		}
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

func issueLabels(i *jira.Issue) []string {
	if i.Fields == nil {
		return nil
	}
	return i.Fields.Labels
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
	key := fmt.Sprintf("%-*s", keyCol, truncate(issueKey(i), keyCol))
	left := typeCell(i) + " " + priorityCell(i) + " " + key + "  " + statusCell(issueStatus(i)) + "  "
	l := layoutFor(width)
	if !l.age {
		return left + issueSummary(i)
	}
	row := left + padRight(truncCells(issueSummary(i), l.sumW), l.sumW)
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

// statusCell renders the status name as a width-padded, colored cell.
func statusCell(status string) string {
	padded := fmt.Sprintf("%-*s", statusCol, truncate(status, statusCol))
	return theme.StatusStyle(status).Render(padded)
}

// statusPill renders the status as a reverse-video pill for detail headers.
func statusPill(status string) string {
	if status == "" {
		return ""
	}
	return theme.StatusStyle(status).Reverse(true).Render(" " + status + " ")
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
	return c + "    " + lipgloss.NewStyle().Faint(true).Render(truncate(lenses[active].JQL, room))
}

// detailHeading renders the shared header for the sidebar and the
// detail view: a faint project breadcrumb with the key, the summary in bold,
// then a status pill, the type badge and a relative updated age. now is passed
// in (matching rowText) so one render frame stamps every age consistently.
func detailHeading(i *jira.Issue, width int, now time.Time, baseURL string) string {
	key := issueKey(i)
	var b strings.Builder
	if proj := projectOf(key); proj != "" {
		b.WriteString(theme.DetailDim.Render(proj + " · "))
	}
	linked := key
	if baseURL != "" && key != "" {
		// Link the bare key and style outside: the hyperlink sanitizer strips
		// ESC bytes, so styling first would shred the SGR sequences.
		linked = adf.Hyperlink(strings.TrimRight(baseURL, "/")+"/browse/"+key, key)
	}
	b.WriteString(theme.DetailHeader.Render(linked) + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(wrap(issueSummary(i), width)) + "\n\n")

	meta := make([]string, 0, 3)
	if pill := statusPill(issueStatus(i)); pill != "" {
		meta = append(meta, pill)
	}
	if tn := issueTypeName(i); tn != "" {
		meta = append(meta, typeCell(i)+" "+tn)
	}
	if ag := age(issueUpdated(i), now); ag != "" {
		meta = append(meta, theme.DetailDim.Render("updated "+ag+" ago"))
	}
	line := strings.Join(meta, "  ")
	if width > 0 && lipgloss.Width(line) > width {
		line = strings.Join(meta, "\n")
	}
	b.WriteString(line + "\n")
	return b.String()
}

// sidebar renders the detail of the selected issue into width columns. Comments
// and the valid-transition list arrive with the action controller (which already
// fetches transitions); this pane shows what the list fetch returns.
func sidebar(i *jira.Issue, width int, md *markdown.Renderer, baseURL string) string {
	if i == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(detailHeading(i, width, time.Now(), baseURL) + "\n")
	fmt.Fprintf(&b, "Assignee:  %s\n", issueAssignee(i))
	if rep := issueReporter(i); rep != "" {
		fmt.Fprintf(&b, "Reporter:  %s\n", rep)
	}
	if p := issuePriority(i); p != "" {
		glyph := priorityGlyphs[p]
		if ps, ok := theme.PriorityStyle(p); ok {
			fmt.Fprintf(&b, "Priority:  %s\n", ps.Render(strings.TrimSpace(glyph+" "+p)))
		} else {
			fmt.Fprintf(&b, "Priority:  %s\n", p)
		}
	}
	if labels := issueLabels(i); len(labels) > 0 {
		fmt.Fprintf(&b, "Labels:    %s\n", strings.Join(labels, ", "))
	}
	if i.Fields != nil && i.Fields.Description != nil {
		fmt.Fprintf(&b, "\n%s\n\n%s\n", theme.DetailHeader.Render("Description"), adfBody(md, issueKey(i)+":desc", i.Fields.Description, width))
	}
	return b.String()
}

// adfBody renders an ADF document for display: ADF → GFM → themed glamour,
// cached under id+width. Nil documents render empty.
func adfBody(md *markdown.Renderer, id string, doc *adf.Document, width int) string {
	if doc == nil {
		return ""
	}
	return md.Render(id, width, adf.ToMarkdown(*doc))
}

// Detail sub-tabs: Overview carries the metadata and the full
// description; Comments carries the conversation.
const (
	detailOverview = iota
	detailComments
	detailTabCount
)

// renderDetail is the full-issue body for the scrollable detail view: header
// and metadata, then the active sub-tab's content — the description on
// Overview, the comments (or a loading placeholder until the full fetch
// lands) on Comments.
func renderDetail(i *jira.Issue, loadingComments bool, width, tab int, md *markdown.Renderer, spin, baseURL string) string {
	if i == nil {
		return ""
	}
	if width < 1 {
		width = 1
	}
	var b strings.Builder
	b.WriteString(detailHeading(i, width, time.Now(), baseURL) + "\n")
	fmt.Fprintf(&b, "Assignee: %s", issueAssignee(i))
	if rep := issueReporter(i); rep != "" {
		fmt.Fprintf(&b, "    Reporter: %s", rep)
	}
	if p := issuePriority(i); p != "" {
		fmt.Fprintf(&b, "    Priority: %s%s", priorityCell(i), " "+p)
	}
	b.WriteString("\n")
	if labels := issueLabels(i); len(labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(labels, ", "))
	}

	if tab == detailComments {
		b.WriteString("\n" + theme.DetailHeader.Render("Comments") + "\n\n")
		if loadingComments {
			b.WriteString(spin + theme.DetailDim.Render(" loading…"))
		} else {
			b.WriteString(renderComments(i, width, md))
		}
		return b.String()
	}

	b.WriteString("\n" + theme.DetailHeader.Render("Description") + "\n\n")
	if i.Fields != nil && i.Fields.Description != nil {
		b.WriteString(adfBody(md, issueKey(i)+":desc", i.Fields.Description, width))
	} else {
		b.WriteString(theme.DetailDim.Render("(no description)"))
	}
	return b.String()
}

// renderComments lists the issue's comments (author, timestamp, body).
func renderComments(i *jira.Issue, width int, md *markdown.Renderer) string {
	if i.Fields == nil || i.Fields.Comment == nil || len(i.Fields.Comment.Comments) == 0 {
		return theme.DetailDim.Render("(no comments)")
	}
	parts := make([]string, 0, len(i.Fields.Comment.Comments))
	for idx, c := range i.Fields.Comment.Comments {
		if c == nil {
			continue
		}
		author := "Unknown"
		if c.Author != nil {
			if n := deref(c.Author.DisplayName); n != "" {
				author = n
			}
		}
		head := theme.DetailLabel.Render(author)
		if when := deref(c.Created); when != "" {
			head += theme.DetailDim.Render("  " + when)
		}
		body := ""
		if c.Body != nil {
			// The index keeps keys distinct even if a comment arrives without
			// an ID — a shared key would serve one comment's body for another.
			id := fmt.Sprintf("%s:comment:%s:%d", issueKey(i), deref(c.ID), idx)
			body = adfBody(md, id, c.Body, width)
		}
		parts = append(parts, head+"\n"+body)
	}
	return strings.Join(parts, "\n\n")
}

// truncate shortens s to n display runes, adding an ellipsis. It counts runes,
// not bytes, so it never splits a multi-byte UTF-8 sequence.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return string(r[:1])
	}
	return string(r[:n-1]) + "…"
}

// wrap is a minimal width-aware wrap on spaces, measuring words in runes so
// Unicode summaries don't break early. width <= 0 leaves text intact.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var (
		out  strings.Builder
		line int
	)
	for i, word := range strings.Fields(s) {
		w := utf8.RuneCountInString(word)
		if i > 0 {
			if line+1+w > width {
				out.WriteByte('\n')
				line = 0
			} else {
				out.WriteByte(' ')
				line++
			}
		}
		out.WriteString(word)
		line += w
	}
	return out.String()
}
