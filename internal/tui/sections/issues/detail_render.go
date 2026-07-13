// Detail rendering shared by the preview sidebar and the
// full-screen detail view: heading, metadata, description, and comments.

package issues

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/gechr/x/human"
	"github.com/gechr/x/ptr"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/markdown"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

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
	if xstrings.AllNonEmpty(baseURL, key) {
		// Link the bare key and style outside: the hyperlink sanitizer strips
		// ESC bytes, so styling first would shred the SGR sequences.
		linked = adf.Hyperlink(strings.TrimRight(baseURL, "/")+"/browse/"+key, key)
	}
	b.WriteString(theme.DetailHeader.Render(linked) + "\n")
	// Wrap the raw text, style after: CodeSpansWith is the last pass so a
	// `span`'s reset can't cut the bold base off mid-heading.
	b.WriteString(theme.CodeSpansWith(wrap(issueSummary(i), width), lipgloss.NewStyle().Bold(true)) + "\n\n")

	meta := make([]string, 0, 3)
	if pill := statusPill(i); pill != "" {
		meta = append(meta, pill)
	}
	if tn := issueTypeName(i); tn != "" {
		meta = append(meta, typeCell(i)+" "+tn)
	}
	if ts := issueUpdated(i); ts != "" {
		if t, err := parseJiraTime(ts); err == nil {
			// The library words the tense ("3d ago", "now"); clock skew
			// clamps to "updated now" rather than claiming a forecast.
			meta = append(meta, theme.DetailDim.Render("updated "+human.FormatTimeAgoCompactFrom(clampFuture(t, now), now)))
		}
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
			if n := ptr.Deref(c.Author.DisplayName); n != "" {
				author = n
			}
		}
		head := theme.DetailLabel.Render(author)
		if when := ptr.Deref(c.Created); when != "" {
			head += theme.DetailDim.Render("  " + when)
		}
		body := ""
		if c.Body != nil {
			// The index keeps keys distinct even if a comment arrives without
			// an ID — a shared key would serve one comment's body for another.
			id := fmt.Sprintf("%s:comment:%s:%d", issueKey(i), ptr.Deref(c.ID), idx)
			body = adfBody(md, id, c.Body, width)
		}
		parts = append(parts, head+"\n"+body)
	}
	return strings.Join(parts, "\n\n")
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
