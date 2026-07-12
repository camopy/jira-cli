// The results list's detail surfaces: the always-visible
// preview pane and the scrollable full-issue detail view with its sub-tabs.

package issues

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gechr/primer/scrollbar"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// barColumn builds a one-glyph-per-row scrollbar (█ thumb, │ track).
func barColumn(height, total int, fraction float64) []string {
	pos, size := scrollbar.ThumbMetrics(height, total, fraction)
	end := pos + size - 1
	col := make([]string, height)
	for i := range col {
		if i >= pos && i <= end {
			col[i] = "█"
		} else {
			col[i] = "│"
		}
	}
	return col
}

// vpWithBar renders a viewport, appending a scrollbar column when it overflows.
func vpWithBar(vp viewport.Model) string {
	body := vp.View()
	h, total := vp.Height(), vp.TotalLineCount()
	if h <= 0 || total <= h {
		return body
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, body, strings.Join(barColumn(h, total, vp.ScrollPercent()), "\n"))
}

// detailWidth is the detail content width, leaving one column for the scrollbar.
func (r *results) detailWidth() int {
	w := r.ctx.ScreenWidth - 1
	if w < 1 {
		w = 1
	}
	return w
}

// detailBody renders the detail viewport with a scrollbar column when the
// content overflows, matching the list's scroll affordance.
func (r *results) detailBody() string { return vpWithBar(r.detail) }

// setDetailTab selects a detail sub-tab (modulo the tab count), re-renders the
// viewport for it and resets the scroll.
func (r *results) setDetailTab(tab int) {
	r.detailTab = ((tab % detailTabCount) + detailTabCount) % detailTabCount
	if r.detailIssue != nil {
		r.detail.SetContent(renderDetail(r.detailIssue, r.detailLoading, r.detailWidth(), r.detailTab, r.md, r.spin.View(), r.ctx.BaseURL))
		r.detail.GotoTop()
	}
}

// detailPills renders the detail view's sub-tab bar (Overview · Comments (n)),
// with the active tab reverse-video.
func (r *results) detailPills() string {
	labels := r.detailPillLabels()
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == r.detailTab {
			parts[i] = r.ctx.Styles.TabActive.Render(l)
		} else {
			parts[i] = r.ctx.Styles.TabInactive.Render(l)
		}
	}
	return strings.Join(parts, " ")
}

// detailPillLabels is the single source for the sub-tab labels, shared by the
// renderer and the click hit-test so their geometry can never drift.
func (r *results) detailPillLabels() []string {
	count := "…"
	if !r.detailLoading {
		n := 0
		if r.detailIssue != nil && r.detailIssue.Fields != nil && r.detailIssue.Fields.Comment != nil {
			n = len(r.detailIssue.Fields.Comment.Comments)
		}
		count = fmt.Sprintf("%d", n)
	}
	return []string{"Overview", "Comments (" + count + ")"}
}

// previewContentWidth is the inner text width of the always-visible sidebar: the
// preview region less the style padding (2) and the scrollbar column (1), and one
// more for the left divider when the sidebar is docked to the right.
func (r *results) previewContentWidth() int {
	w := r.ctx.PreviewWidth - 3
	if pos := r.ctx.PreviewPosition(); pos == core.PreviewRight || pos == core.PreviewLeft {
		w-- // side docks draw a one-column divider
	}
	if w < 1 {
		w = 1
	}
	return w
}

// syncPreview points the preview viewport at the current selection, sized to the
// sidebar region. It rebuilds the content only when the selected issue or the
// width changes (and scrolls to the top only on a selection change), so scrolling
// within a long description survives the per-frame re-render. Use on navigation
// and resize, where the selected issue's data is unchanged.
func (r *results) syncPreview() { r.syncPreviewContent(false) }

// refreshPreview forces a content rebuild for the current selection, used after
// the underlying data may have changed for the same selected issue (a refetch or
// an optimistic transition) — where syncPreview's key/width guard would wrongly
// keep stale content. The scroll offset is preserved unless the selection itself
// changed.
func (r *results) refreshPreview() { r.syncPreviewContent(true) }

func (r *results) syncPreviewContent(force bool) {
	w := r.previewContentWidth()
	r.preview.SetWidth(w)
	h := r.ctx.PreviewHeight
	if r.ctx.PreviewPosition() == core.PreviewBottom {
		h-- // the bottom dock's top divider takes one row of the region
	}
	if h < 0 {
		h = 0
	}
	r.preview.SetHeight(h)
	sel := r.selected()
	key := ""
	if sel != nil {
		key = issueKey(sel)
	}
	changed := key != r.previewKey
	if !force && !changed && w == r.previewW {
		return
	}
	r.preview.SetContent(sidebar(sel, w, r.md, r.ctx.BaseURL))
	if changed {
		r.preview.GotoTop()
	}
	r.previewKey, r.previewW = key, w
}

// openDetail shows the scrollable detail view for an issue immediately (with the
// data already in the list), then fetches the full issue — description and
// comments — to fill it in.
func (r *results) openDetail(iss *jira.Issue) tea.Cmd {
	r.ctx.Recent.Touch(issueKey(iss), issueSummary(iss))
	r.detailing = true
	r.detailLoading = true
	r.detailTab = detailOverview
	r.detailIssue = iss
	r.detail.SetWidth(r.detailWidth())
	r.detail.GotoTop()
	r.detail.SetContent(renderDetail(iss, true, r.detailWidth(), r.detailTab, r.md, r.spin.View(), r.ctx.BaseURL))
	base := r.ctx.Base
	svc := r.ctx.Services
	key := issueKey(iss)
	return tea.Batch(r.spin.Tick, r.ctx.StartTask(core.TaskSpec{
		Scope: r.detailScope(),
		Run: func() (any, error) {
			if svc == nil {
				return detailResult{issue: iss}, nil
			}
			full, _, err := svc.Issues().Get(base, key, &jira.IssueGetOptions{})
			if err != nil {
				return nil, err
			}
			return detailResult{issue: full}, nil
		},
	}))
}

// previewScrollable reports that the always-visible preview is on screen, so the
// page keys and wheel should scroll it rather than the list.
func (r *results) previewScrollable() bool {
	return r.ctx.SidebarOpen && r.ctx.PreviewWidth > 0
}

// updateDetail scrolls the detail viewport, closes it on esc/enter, or quits.
// Quit must be handled here because the detail view captures input (capturing()),
// which otherwise suppresses the App-level quit shortcut and would strand the
// user in the detail view until they pressed esc.
func (r *results) updateDetail(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, r.ctx.Keys.Quit):
		return tea.Quit
	case key.Matches(msg, r.ctx.Keys.Back), key.Matches(msg, r.ctx.Keys.Open):
		r.detailing = false
		r.detailIssue = nil
		return nil
	case key.Matches(msg, r.ctx.Keys.NextSection):
		// tab/shift+tab cycle the Overview/Comments sub-tabs (the detail view is
		// modal, so the section-switch keys are free here).
		r.setDetailTab(r.detailTab + 1)
		return nil
	case key.Matches(msg, r.ctx.Keys.PrevSection):
		r.setDetailTab(r.detailTab + detailTabCount - 1)
		return nil
	case key.Matches(msg, r.ctx.Keys.OpenBrowse):
		if r.detailIssue != nil {
			return r.openInBrowser(issueKey(r.detailIssue))
		}
		return nil
	}
	var cmd tea.Cmd
	r.detail, cmd = r.detail.Update(msg)
	return cmd
}
