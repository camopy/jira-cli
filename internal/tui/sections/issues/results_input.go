// Input routing for the results list: key, wheel, click,
// and paste handling, plus the hit-tests that map screen positions to rows.

package issues

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// handleKey processes the navigation, filter, and action keys common to every
// issue list. It returns true when it consumed the key, so the owner can fall
// back to its own section-specific bindings.
func (r *results) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if r.detailing {
		return r.updateDetail(msg), true
	}
	// The dialog stack is modal: the text/create form, a facet/jumplist/
	// transition pick, or a bulk confirmation gets keys before the filter
	// routing, mirroring how the App routes its own stack ahead of section input.
	if r.dialogs.Active() {
		return r.updateDialog(msg), true
	}
	if r.filtering {
		return r.updateFilter(msg), true
	}
	k := r.ctx.Keys
	// moved marks cursor-movement keys: only those may trigger a fetch-more,
	// so a verb pressed while parked on the last row can't start a background
	// page fetch under an opening modal.
	moved := false
	switch {
	case key.Matches(msg, k.Open):
		// Enter opens the detail view for the selected row. With no selection it
		// is not consumed, so a section (Search) can use Enter for its own thing.
		if iss := r.selected(); iss != nil {
			return r.openDetail(iss), true
		}
		return nil, false
	case key.Matches(msg, k.Down):
		r.list.MoveDown(1)
		moved = true
	case key.Matches(msg, k.Up):
		r.list.MoveUp(1)
	case key.Matches(msg, k.Top):
		r.list.Top()
	case key.Matches(msg, k.Bottom):
		r.list.Bottom()
		moved = true
	case key.Matches(msg, k.PageDown):
		// With the sidebar visible PageUp/PageDown scroll the preview (j/k still
		// move the list); with it hidden they page the list, so the keys are never
		// dead. Call the viewport's page method directly rather than re-feeding the
		// key through its own keymap, so a rebound page key still scrolls.
		if r.previewScrollable() {
			r.preview.PageDown()
			return nil, true
		}
		r.list.PageDown()
		moved = true
	case key.Matches(msg, k.PageUp):
		if r.previewScrollable() {
			r.preview.PageUp()
			return nil, true
		}
		r.list.PageUp()
	case key.Matches(msg, k.Facet):
		r.openFacets()
	case key.Matches(msg, k.Jumplist):
		r.openJumplist()
	case key.Matches(msg, k.Filter):
		r.filtering = true
		r.filterInput = input.NewLine("/", "")
		r.filterInput.SetWidth(r.ctx.MainWidth - 2)
		r.filterInput.SetValue(r.filter)
	case key.Matches(msg, k.Select):
		if iss := r.selected(); iss != nil {
			r.toggleMark(issueKey(iss))
			r.markAnchor = issueKey(iss)
			r.rebuildRows() // only the marker column changed
		}
	case key.Matches(msg, k.SelectAll):
		r.selectAllShown()
		r.rebuildRows()
	case key.Matches(msg, k.SelectInvert):
		r.invertShown()
		r.rebuildRows()
	case key.Matches(msg, k.SelectRange):
		r.selectRange()
		r.rebuildRows()
	case key.Matches(msg, k.Transition):
		if !r.canMutate() {
			return nil, true // another write is still reconciling
		}
		if keys := r.markedKeys(); len(keys) > 0 {
			// The choices come from the first marked issue; mixed workflows
			// surface as per-issue failures in the bulk result.
			return r.fetchTransitions(keys[0], true), true
		}
		if iss := r.selected(); iss != nil {
			return r.fetchTransitions(issueKey(iss), false), true
		}
	case key.Matches(msg, k.Comment):
		// With a selection the comment goes to every marked issue through the
		// in-modal textarea (the external-editor flow stays single-issue).
		if len(r.markedKeys()) > 0 && r.canMutate() {
			r.openTextForm(action.ModeBulkComment, "", "")
			return nil, true
		}
		return r.openComment(), true
	case key.Matches(msg, k.Edit):
		if iss := r.selected(); iss != nil {
			r.openTextVerb(action.ModeEdit, issueSummary(iss))
		}
	case key.Matches(msg, k.Assign):
		if len(r.markedKeys()) > 0 && r.canMutate() {
			r.openTextForm(action.ModeBulkAssign, "", "")
			return nil, true
		}
		r.openTextVerb(action.ModeAssign, "")
	case key.Matches(msg, k.Worklog):
		r.openTextVerb(action.ModeWorklog, "")
	case key.Matches(msg, k.Labels):
		// Pre-filled with the current labels so the edit is a round-trip:
		// submitting the emptied field deliberately clears them all.
		if iss := r.selected(); iss != nil {
			r.openTextVerb(action.ModeLabels, strings.Join(issueLabels(iss), ", "))
		}
	case key.Matches(msg, k.Create):
		return r.openCreate(), true
	case key.Matches(msg, k.AssignMe):
		if iss := r.selected(); iss != nil && r.canMutate() {
			// "me" is a placeholder; the reconcile swaps in the real name.
			r.rollback = r.applyOptimisticAssignee(issueKey(iss), "me")
			return r.assignMe(issueKey(iss)), true
		}
	case key.Matches(msg, k.OpenBrowse):
		if iss := r.selected(); iss != nil {
			return r.openInBrowser(issueKey(iss)), true
		}
	case key.Matches(msg, k.CopyKey):
		if iss := r.selected(); iss != nil {
			return tea.Batch(tea.SetClipboard(issueKey(iss)), r.flashNotice("copied "+issueKey(iss), false)), true
		}
	case key.Matches(msg, k.CopyURL):
		if iss := r.selected(); iss != nil {
			if url := r.issueURL(issueKey(iss)); url != "" {
				return tea.Batch(tea.SetClipboard(url), r.flashNotice("copied url", false)), true
			}
		}
	default:
		return nil, false
	}
	// A cursor move (or any consumed key that may have changed the selection)
	// repoints the preview at the now-selected issue; a downward move landing
	// on the last loaded row pulls the next page if there is one.
	r.syncPreview()
	r.viewSelected()
	if moved {
		return r.maybeFetchMore(), true
	}
	return nil, true
}

// handleWheel routes a wheel event to the pane under the pointer: the detail
// overlay when open (it covers the body), else the preview when the pointer is
// over it, and otherwise the list — so the wheel scrolls what the user is
// pointing at and is never dead.
func (r *results) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if r.modalOpen() {
		// The overlays cover the body and have no scroll; moving the list
		// cursor (and clearing its change marks) under them would be wrong.
		return nil
	}
	var cmd tea.Cmd
	switch {
	case r.detailing:
		r.detail, cmd = r.detail.Update(msg)
	case r.wheelOverPreview(msg.X, msg.Y):
		r.preview, cmd = r.preview.Update(msg)
	case msg.Button == tea.MouseWheelDown:
		r.list.MoveDown(1)
		r.viewSelected()
		return r.maybeFetchMore()
	case msg.Button == tea.MouseWheelUp:
		r.list.MoveUp(1)
		r.viewSelected()
	}
	return cmd
}

// wheelOverPreview hit-tests a mouse position against the preview region for
// the current dock, so the wheel scrolls the pane under the pointer. Y is an
// absolute screen row; the bottom dock offsets it by the App's top chrome.
func (r *results) wheelOverPreview(x, y int) bool {
	if !r.previewScrollable() {
		return false
	}
	switch r.ctx.PreviewPosition() {
	case core.PreviewRight:
		return x >= r.ctx.MainWidth
	case core.PreviewLeft:
		return x < r.ctx.PreviewWidth
	default: // bottom
		return y >= core.TopChromeRows+r.ctx.MainHeight
	}
}

// handleClick routes a left click: detail sub-tab pills while the detail view
// is open; otherwise a list click selects the row under the pointer, and a
// click on the already-selected row opens its detail (a click-twice "double
// click" that needs no timing). Clicks under an open modal are ignored.
func (r *results) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft || r.modalOpen() {
		return nil
	}
	if r.detailing {
		if msg.Y == core.TopChromeRows+r.headerRows {
			if tab, ok := r.pillAt(msg.X); ok {
				r.setDetailTab(tab)
			}
		}
		return nil
	}
	idx, ok := r.rowAt(msg.X, msg.Y)
	if !ok {
		return nil
	}
	if idx == r.list.Cursor() {
		if iss := r.selected(); iss != nil {
			return r.openDetail(iss)
		}
		return nil
	}
	r.list.SetCursor(idx)
	r.syncPreview()
	r.viewSelected()
	return r.maybeFetchMore()
}

// listTop is the absolute screen row of the first visible list row: the App
// chrome, the owner header (as rendered last frame), the status line and the
// column header sit above it.
func (r *results) listTop() int { return core.TopChromeRows + r.headerRows + 2 }

// mainX translates an absolute click column into a main-pane column,
// reporting false when the click is inside a docked preview pane.
func (r *results) mainX(x int) (int, bool) {
	if r.ctx.SidebarOpen && r.ctx.PreviewWidth > 0 {
		switch r.ctx.PreviewPosition() {
		case core.PreviewLeft:
			if x < r.ctx.PreviewWidth {
				return 0, false
			}
			return x - r.ctx.PreviewWidth, true
		case core.PreviewRight:
			if x >= r.ctx.MainWidth {
				return 0, false
			}
		}
	}
	return x, true
}

// rowAt maps a screen position to a list row index, reporting false for
// clicks outside the list (chrome, preview pane, past the last row).
func (r *results) rowAt(x, y int) (int, bool) {
	if _, ok := r.mainX(x); !ok {
		return 0, false
	}
	row := y - r.listTop()
	if row < 0 {
		return 0, false
	}
	start, end := r.list.VisibleRange()
	idx := start + row
	if idx >= end {
		return 0, false
	}
	return idx, true
}

// pillAt maps a click x on the detail sub-tab row to a tab index by walking
// the rendered pill widths (labels joined by one space). It measures the
// styled labels, so style padding counts toward the clickable cell.
func (r *results) pillAt(x int) (int, bool) {
	pos := 0
	for i, label := range r.detailPillLabels() {
		style := r.ctx.Styles.TabInactive
		if i == r.detailTab {
			style = r.ctx.Styles.TabActive
		}
		w := lipgloss.Width(style.Render(label))
		if x >= pos && x < pos+w {
			return i, true
		}
		pos += w + 1
	}
	return 0, false
}

// updateFilter drives the live filter through the shared input: every edit
// (including cursor-position inserts and paste) re-applies immediately, esc
// clears, enter keeps the filter and returns focus to the list.
func (r *results) updateFilter(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		r.filtering = false
		r.filter = ""
		r.applyFilter()
	case "enter":
		r.filtering = false
	default:
		return r.syncFilter(msg)
	}
	return nil
}

// syncFilter feeds a message into the filter input and re-applies the filter
// when the text changed, returning the input's cmd (cursor blink etc.).
func (r *results) syncFilter(msg tea.Msg) tea.Cmd {
	cmd := r.filterInput.Update(msg)
	if v := r.filterInput.Value(); v != r.filter {
		r.filter = v
		r.applyFilter()
	}
	return cmd
}

// handlePaste routes a bracketed-paste payload into whichever input has
// focus; the boolean reports whether anything consumed it.
func (r *results) handlePaste(msg tea.PasteMsg) (tea.Cmd, bool) {
	switch {
	case r.dialogs.Active():
		return r.updateDialog(msg), true
	case r.filtering:
		return r.syncFilter(msg), true
	}
	return nil, false
}

// openTextVerb opens a free-text action against the selected issue, if any and
// if no other mutation is in flight.
func (r *results) openTextVerb(mode action.Mode, initial string) {
	if iss := r.selected(); iss != nil && r.canMutate() {
		r.openTextForm(mode, issueKey(iss), initial)
	}
}
