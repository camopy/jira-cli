// Package issues — row materialization for the results list: rebuilding the
// visible rows from the filtered set and maintaining the multi-select marks.
package issues

import (
	"time"

	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// rebuildRows re-renders the visible rows from r.shown: the 2-cell marker
// column carries the bulk-select check first, else the change dot (green for
// rows a refresh added, yellow for ones it modified) until the row is viewed.
func (r *results) rebuildRows() {
	// Row budget: the list width less the 2-char marker column and the
	// scrollbar column listviewport reserves on overflow.
	rowW := r.ctx.MainWidth - 3
	now := time.Now()
	rows := make([]string, len(r.shown))
	for i, iss := range r.shown {
		marker := "  "
		switch key := issueKey(iss); {
		case r.marks[key]:
			marker = "✓ "
		case r.changed[key] == changeNew:
			marker = theme.StatusDone.Render("●") + " "
		case r.changed[key] == changeUpdated:
			marker = theme.StatusInProgress.Render("●") + " "
		}
		rows[i] = marker + rowText(iss, rowW, now)
	}
	r.list.SetRows(rows)
}

func (r *results) applyFilter() {
	r.shown = applyFacet(filterIssues(r.all, r.filter), r.facet)
	r.rebuildRows()
	// Data may have changed for the still-selected issue (refetch / optimistic
	// transition), so force a rebuild rather than trusting the key/width guard.
	r.refreshPreview()
}

// markedKeys returns the selected issue keys still present in the result set, in
// list order so the bulk action is deterministic. It deliberately walks r.all,
// not r.shown: a selection survives a local filter change, so the user can mark,
// narrow the filter to find more, mark those too, then act on the whole set. The
// status-line count reflects the same total. Marks for issues that dropped out
// of a refetch are pruned by pruneMarks, so this never returns a vanished key.
func (r *results) markedKeys() []string {
	if len(r.marks) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.marks))
	for _, iss := range r.all {
		if r.marks[issueKey(iss)] {
			out = append(out, issueKey(iss))
		}
	}
	return out
}

// pruneMarks drops selections for issues no longer in the list (e.g. after a
// refetch changed the result set), so the selection count and the bulk target
// can never disagree.
func (r *results) pruneMarks() {
	if len(r.marks) == 0 {
		return
	}
	present := make(map[string]bool, len(r.all))
	for _, iss := range r.all {
		present[issueKey(iss)] = true
	}
	for k := range r.marks {
		if !present[k] {
			delete(r.marks, k)
		}
	}
}

// toggleMark adds or removes an issue from the multi-selection.
func (r *results) toggleMark(key string) {
	if key == "" {
		return
	}
	if r.marks == nil {
		r.marks = make(map[string]bool)
	}
	if r.marks[key] {
		delete(r.marks, key)
	} else {
		r.marks[key] = true
	}
}
