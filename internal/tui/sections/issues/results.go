package issues

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gechr/primer/flash"
	"github.com/gechr/primer/overlay"
	"github.com/gechr/primer/scrollbar"
	xmaps "github.com/gechr/x/maps"
	"github.com/gechr/x/ptr"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"

	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/components/listviewport"
	"github.com/matcra587/jira-cli/internal/tui/components/markdown"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// workdaySeconds is the assumed length of a working day for parsing relative
// worklog durations like "1d". Jira's default is 8 hours.
const workdaySeconds = 8 * 60 * 60

// fetchResult, transitionsResult, and bulkResult are delivered as
// TaskFinishedMsg.Result.
type (
	// fetchResult delivers one page of issues plus the opaque continuation
	// cursor, so the list can fetch more when the user scrolls to the bottom.
	fetchResult struct {
		issues []*jira.Issue
		cursor jira.PageCursor
	}
	// fetchMoreResult appends a follow-up page to the existing result set.
	fetchMoreResult struct {
		issues []*jira.Issue
		cursor jira.PageCursor
	}
	transitionsResult struct {
		issueKey    string
		transitions []*jira.Transition
		// bulk routes the result to the bulk picker: the fetch ran against one
		// representative marked issue and the pick applies to the selection.
		bulk bool
	}
	// bulkResult reports the outcome of a bulk write. A per-issue failure
	// (e.g. the target status isn't reachable for that issue) is recorded here
	// rather than aborting the batch, so one bad issue can't strand the rest.
	bulkResult struct {
		verb      string // preformatted outcome suffix, e.g. `→ Done`, `commented`
		succeeded []string
		failed    map[string]error
	}
	// detailResult delivers the fully-fetched issue (description + comments) for
	// the detail view.
	detailResult struct{ issue *jira.Issue }
)

// summary is the one-line status shown after a bulk write lands. On a
// partial failure it names the issues that didn't take, so the user can retry
// or inspect them rather than being told only a count.
func (b bulkResult) summary() string {
	if len(b.failed) == 0 {
		return fmt.Sprintf("%d %s", len(b.succeeded), b.verb)
	}
	keys := xmaps.KeysNatural(b.failed)
	return fmt.Sprintf("%d %s · failed: %s", len(b.succeeded), b.verb, strings.Join(keys, ", "))
}

// results is the list + sidebar + local filter + action controller shared by the
// Issues and Search sections. It owns everything except the query source: the
// owning section decides what JQL to run and renders its own header, then hands
// the fetched issues here. refetch lets a confirmed mutation reconcile with the
// server using the owner's current query.
// changeKind classifies what a refresh did to a row.
type changeKind int

const (
	changeNone changeKind = iota
	changeNew
	changeUpdated
)

type results struct {
	ctx     *core.ProgramContext
	id      core.SectionID // namespaces task scopes so sections don't collide
	list    *listviewport.Model
	refetch func() tea.Cmd

	all   []*jira.Issue
	shown []*jira.Issue

	// marks is the multi-select set, keyed by issue key. Marks survive a refetch
	// only for issues that are still present; markedKeys filters against all.
	// markAnchor is the last space-toggled key, the start of a range select.
	marks      map[string]bool
	markAnchor string

	// changed marks rows a background refresh added or modified, keyed by
	// issue key, until the user views them; seen is the key→updated snapshot
	// the next refresh diffs against.
	changed map[string]changeKind
	seen    map[string]string

	filter      string
	filtering   bool
	filterInput input.Line

	// facet narrows the rows to one status/assignee/label value on top of
	// the text filter; facetPick is the open picker while faceting.
	facet        facet
	facetPick    picker.Model
	facetChoices facetChoices
	faceting     bool

	// jumpPick is the recent-issues jumplist picker while jumping.
	jumpPick   picker.Model
	jumping    bool
	md         *markdown.Renderer // cached, theme-styled body renderer
	headerRows int                // owner header height (set by applySize), for click hit-tests
	loading    bool
	fetched    bool // a fetch has landed at least once; gates the tab-bar count
	err        error
	flash      flash.State   // transient action feedback, auto-cleared
	spin       spinner.Model // animates while a fetch is in flight

	// Pagination for the current query: lastJQL is what runFetch last ran
	// (fetch-more must repeat it verbatim), cursor is the opaque continuation
	// handle from the last page, and loadingMore guards against stacking page
	// fetches while one is in flight.
	lastJQL     string
	cursor      jira.PageCursor
	loadingMore bool

	ctrl        action.Controller
	confirm     *bulkConfirm // a bulk request awaiting its y/N prompt
	rollback    func()
	bulkPending bool // a bulk write is in flight; blocks re-entry
	writing     bool // a single-issue write (comment/assign/edit/worklog) is in flight

	// preview is the always-visible issue detail beside/below the list; it
	// scrolls independently (PageUp/PageDown) for issues with long descriptions.
	// previewKey/previewW track the issue and width its content was last rendered
	// for, so syncPreview rebuilds (and scrolls to top) only when the selection or
	// layout actually changes — leaving the scroll offset alone between frames so
	// PageUp/PageDown stick.
	preview    viewport.Model
	previewKey string
	previewW   int

	// detail is the scrollable full-issue view, shown over the list while
	// detailing is true. detailTab selects the Overview/Comments sub-tab.
	detailing     bool
	detailLoading bool
	detailTab     int
	detailIssue   *jira.Issue
	detail        viewport.Model
}

func newResults(ctx *core.ProgramContext, id core.SectionID) results {
	return results{
		ctx: ctx, id: id,
		list: listviewport.New(), preview: viewport.New(), detail: viewport.New(),
		md:   markdown.NewRenderer(markdown.StyleFromTheme(theme.Theme)),
		spin: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(theme.StatusInProgress)),
	}
}

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

// Task scopes are namespaced by section ID so Issues and Search — both built and
// both receiving the App's broadcast TaskFinishedMsgs — never apply each other's
// results or share a generation counter.
func (r *results) fetchScope() core.TaskScope { return core.TaskScope(string(r.id) + ".fetch") }

func (r *results) transitionsScope() core.TaskScope {
	return core.TaskScope(string(r.id) + ".transitions")
}
func (r *results) mutateScope() core.TaskScope { return core.TaskScope(string(r.id) + ".mutate") }
func (r *results) detailScope() core.TaskScope { return core.TaskScope(string(r.id) + ".detail") }

// capturing reports that keys must reach this component directly (filter editing,
// an open action, or the scrollable detail view), bypassing global shortcuts.
func (r *results) capturing() bool {
	return r.filtering || r.detailing || r.modalOpen()
}

// modalOpen reports that an overlay covers the body (action prompt, facet or
// jumplist picker, bulk confirmation), so list-targeted mouse events must not
// act on the rows underneath it.
func (r *results) modalOpen() bool {
	return r.ctrl.Active() || r.faceting || r.jumping || r.confirm != nil
}

// flashDuration is how long a transient status toast stays up.
const flashDuration = 4 * time.Second

// flashClearMsg is a flash clear tick addressed to its originating section:
// every section sees it (core broadcasts SectionMsg), only the owner clears,
// so two sections' flash counters can never wipe each other's toasts.
type flashClearMsg struct {
	id    core.SectionID
	clear flash.ClearMsg
}

// Section implements core.SectionMsg.
func (m flashClearMsg) Section() core.SectionID { return m.id }

// flashNotice shows a transient status message and returns the tick that
// clears it. Setting a newer flash invalidates the older clear tick.
func (r *results) flashNotice(msg string, isErr bool) tea.Cmd {
	clear := flashClearMsg{id: r.id, clear: r.flash.Set(msg, isErr)}
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return clear })
}

// handleFlashClear clears the toast when the tick is ours.
func (r *results) handleFlashClear(msg flashClearMsg) {
	if msg.id == r.id {
		r.flash.Clear(msg.clear)
	}
}

// busy reports that a fetch is in flight somewhere in the section, which is
// what keeps the spinner ticking.
func (r *results) busy() bool { return r.loading || r.loadingMore || r.detailLoading }

// handleSpinner advances the spinner while busy; an idle section drops the
// tick, ending the stream until the next fetch starts one.
func (r *results) handleSpinner(msg spinner.TickMsg) tea.Cmd {
	if !r.busy() {
		return nil
	}
	var cmd tea.Cmd
	r.spin, cmd = r.spin.Update(msg)
	// The detail body is a cached viewport snapshot; while its comments are
	// still loading, re-render so the embedded spinner actually animates.
	if r.detailing && r.detailLoading {
		r.detail.SetContent(renderDetail(r.detailIssue, true, r.detailWidth(), r.detailTab, r.md, r.spin.View(), r.ctx.BaseURL))
	}
	return cmd
}

// runFetch loads the first page of a JQL query as a generation-tracked task.
// The owner supplies the query; the result is applied here via
// handleTask(fetchScope), which also records the pagination token.
func (r *results) runFetch(jql string) tea.Cmd {
	if jql != r.lastJQL {
		// A different query is a new world, not a refresh — diffing its
		// results against the old query's snapshot would mark everything.
		r.seen, r.changed = nil, nil
	}
	r.loading = true
	r.loadingMore = false
	r.lastJQL = jql
	base := r.ctx.Base
	svc := r.ctx.Services
	return tea.Batch(r.spin.Tick, r.ctx.StartTask(core.TaskSpec{
		Scope: r.fetchScope(),
		Run: func() (any, error) {
			if svc == nil {
				return fetchResult{}, nil
			}
			issues, next, err := jira.ListIssuesPage(base, svc.Issues(), &jira.IssueListOptions{
				JQL:         jql,
				Fields:      fetchFields,
				ListOptions: jira.ListOptions{MaxResults: 50},
			}, jira.PageCursor{})
			if err != nil {
				return nil, err
			}
			return fetchResult{issues: issues, cursor: next}, nil
		},
	}))
}

// maybeFetchMore starts the next page when the cursor sits on the last loaded
// row and Jira reported more results — scroll-to-fetch. It is a
// no-op while any fetch is in flight, mid-pagination, or on the final page.
func (r *results) maybeFetchMore() tea.Cmd {
	if !r.cursor.More() || r.loading || r.loadingMore {
		return nil
	}
	if r.list.Cursor() != len(r.shown)-1 || len(r.shown) == 0 {
		return nil
	}
	r.loadingMore = true
	jql, cur := r.lastJQL, r.cursor
	base := r.ctx.Base
	svc := r.ctx.Services
	return tea.Batch(r.spin.Tick, r.ctx.StartTask(core.TaskSpec{
		Scope: r.fetchScope(), // same scope: a new first-page fetch supersedes this
		Run: func() (any, error) {
			if svc == nil {
				return fetchMoreResult{}, nil
			}
			issues, next, err := jira.ListIssuesPage(base, svc.Issues(), &jira.IssueListOptions{
				JQL:         jql,
				Fields:      fetchFields,
				ListOptions: jira.ListOptions{MaxResults: 50},
			}, cur)
			if err != nil {
				return nil, err
			}
			return fetchMoreResult{issues: issues, cursor: next}, nil
		},
	}))
}

// applySize sizes the list within the main content region, reserving rows for
// the owner's header lines plus the two shared rows view() always draws: the
// status line and the column header.
func (r *results) applySize(reservedHeaderRows int) {
	r.headerRows = reservedHeaderRows // click hit-tests share the layout's header budget
	h := r.ctx.MainHeight - reservedHeaderRows - 2
	if h < 0 {
		h = 0
	}
	r.list.SetSize(r.ctx.MainWidth, h)

	// The detail view fills the body, reserving rows for its sub-tab pills and
	// hint line, plus one column for the scrollbar.
	dh := r.ctx.BodyHeight - reservedHeaderRows - 2
	if dh < 0 {
		dh = 0
	}
	r.detail.SetWidth(r.detailWidth())
	r.detail.SetHeight(dh)
	if r.detailIssue != nil {
		r.detail.SetContent(renderDetail(r.detailIssue, r.detailLoading, r.detailWidth(), r.detailTab, r.md, r.spin.View(), r.ctx.BaseURL))
	}
	// Rows embed the width (right-aligned age column), so a resize must rebuild
	// them. applyFilter's trailing refreshPreview() also re-sizes and re-renders
	// the preview pane, covering what the old explicit syncPreview() did here.
	r.applyFilter()
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

// Count reports the loaded issue total for the tab bar (core.Counter); ok is
// false until the first fetch lands, so an unvisited section shows no count.
func (r *results) Count() (int, bool) { return len(r.all), r.fetched }

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

// diffChanges compares an incoming refresh against the last snapshot and
// marks rows that are new or carry a newer updated timestamp. The first
// fetch only seeds the snapshot — flagging the whole landing page as "new"
// would be noise.
func (r *results) diffChanges(incoming []*jira.Issue) {
	prev := r.seen
	page := make(map[string]string, len(incoming))
	for _, iss := range incoming {
		page[issueKey(iss)] = issueUpdated(iss)
	}
	if prev != nil {
		for _, iss := range incoming {
			key := issueKey(iss)
			was, known := prev[key]
			switch {
			case !known:
				r.markChanged(key, changeNew)
			case was != issueUpdated(iss):
				r.markChanged(key, changeUpdated)
			}
		}
		// Drop marks for rows that left the page: they aren't renderable, so
		// the map would only grow without bound.
		for key := range r.changed {
			if _, ok := page[key]; !ok {
				delete(r.changed, key)
			}
		}
	}
	// The next snapshot is the page plus everything previously seen that the
	// page doesn't override — a paginated-in issue that re-ranks onto page
	// one later must not read as "new". The snapshot resets with the query.
	for key, was := range prev {
		if _, ok := page[key]; !ok {
			page[key] = was
		}
	}
	r.seen = page
}

func (r *results) markChanged(key string, kind changeKind) {
	if r.changed == nil {
		r.changed = make(map[string]changeKind)
	}
	r.changed[key] = kind
}

// viewSelected clears the selected row's change dot — deliberate navigation
// onto a row counts as viewing it — and repaints when one cleared. It is
// called from user-driven selection paths only, never from refresh-side
// preview syncs, where a stale cursor index could wipe an unviewed mark.
func (r *results) viewSelected() {
	if sel := r.selected(); sel != nil && r.clearChanged(issueKey(sel)) {
		r.rebuildRows()
	}
}

// clearChanged drops an issue's change mark, reporting whether one cleared
// (the rows then need a rebuild for the dot to disappear).
func (r *results) clearChanged(key string) bool {
	if key == "" || r.changed[key] == changeNone {
		return false
	}
	delete(r.changed, key)
	return true
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

// selectKey moves the cursor to the issue with the given key if it is still in
// the filtered view, and repoints the preview. A miss (or empty key) leaves the
// clamped cursor where applyFilter put it.
func (r *results) selectKey(key string) {
	if key == "" {
		return
	}
	for i, iss := range r.shown {
		if issueKey(iss) == key {
			r.list.SetCursor(i)
			r.syncPreview()
			return
		}
	}
}

func (r *results) selected() *jira.Issue {
	c := r.list.Cursor()
	if c < 0 || c >= len(r.shown) {
		return nil
	}
	return r.shown[c]
}

func (r *results) find(key string) *jira.Issue {
	for _, iss := range r.all {
		if issueKey(iss) == key {
			return iss
		}
	}
	return nil
}

// handleTask applies the shared task results (list fetch, transitions, mutation
// outcome). It returns true when it owned the scope.
func (r *results) handleTask(msg core.TaskFinishedMsg) (tea.Cmd, bool) {
	switch msg.Scope {
	case r.fetchScope():
		// Any landing on this scope — first page, follow-up page, or an error
		// from either — ends both in-flight states. Resetting loadingMore here
		// (not just in the fetchMoreResult arm) is what unblocks pagination
		// after a failed page fetch or a superseding first-page fetch.
		r.loading = false
		r.loadingMore = false
		if msg.Err != nil {
			r.err = msg.Err
			return nil, true
		}
		r.err = nil
		switch res := msg.Result.(type) {
		case fetchResult:
			sel := ""
			if cur := r.selected(); cur != nil {
				sel = issueKey(cur)
			}
			r.diffChanges(res.issues)
			r.all = res.issues
			r.fetched = true
			r.cursor = res.cursor
			r.pruneMarks()
			r.applyFilter()
			// Re-find the previously selected issue by key: a refresh (especially a
			// background one) may reorder the results, and keeping the cursor by
			// index would silently swap the issue the user is about to act on.
			r.selectKey(sel)
		case fetchMoreResult:
			// Pagination loads older issues, not changes — register them in the
			// snapshot so the next refresh doesn't call them new, but don't mark.
			if r.seen == nil {
				r.seen = make(map[string]string, len(res.issues))
			}
			for _, iss := range res.issues {
				r.seen[issueKey(iss)] = issueUpdated(iss)
			}
			r.all = append(r.all, res.issues...)
			r.cursor = res.cursor
			// Appending can't move existing rows, so the cursor stays put; the new
			// rows simply extend the list below it.
			r.applyFilter()
		}
		return nil, true
	case r.transitionsScope():
		if msg.Err != nil {
			r.err = msg.Err
			return nil, true
		}
		if res, ok := msg.Result.(transitionsResult); ok {
			if len(res.transitions) == 0 {
				return r.flashNotice("no transitions available", false), true
			}
			if res.bulk {
				r.ctrl.OpenBulkTransition(res.transitions)
			} else {
				r.ctrl.OpenTransition(res.issueKey, res.transitions)
			}
		}
		return nil, true
	case r.mutateScope():
		r.bulkPending = false // any mutate completion ends the in-flight batch
		r.writing = false
		if msg.Err != nil {
			if r.rollback != nil {
				r.rollback()
				r.rollback = nil
			}
			// The optimistic change is already rolled back; the toast tells
			// the user why, then clears — a write error is transient state,
			// not a sticky section error.
			return r.flashNotice("✗ "+msg.Err.Error(), true), true
		}
		r.rollback = nil
		r.err = nil
		// A bulk transition reports per-issue outcomes in its result (the task
		// itself never errors on a partial failure); show the tally and drop the
		// selection before reconciling with the server.
		var note tea.Cmd
		if res, ok := msg.Result.(bulkResult); ok {
			note = r.flashNotice(res.summary(), false)
			r.marks = nil
		}
		if r.refetch != nil {
			return tea.Batch(note, r.refetch()), true
		}
		return note, true
	case r.detailScope():
		r.detailLoading = false
		if msg.Err != nil {
			r.err = msg.Err
			return nil, true
		}
		if res, ok := msg.Result.(detailResult); ok && res.issue != nil {
			r.detailIssue = res.issue
			// Re-touch with the fetched summary: a foreign-key jump opened a
			// bare stub, and a rename since the last visit is stale otherwise.
			r.ctx.Recent.Touch(issueKey(res.issue), issueSummary(res.issue))
			r.detail.SetContent(renderDetail(res.issue, false, r.detailWidth(), r.detailTab, r.md, r.spin.View(), r.ctx.BaseURL))
		}
		return nil, true
	}
	return nil, false
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

// handleKey processes the navigation, filter, and action keys common to every
// issue list. It returns true when it consumed the key, so the owner can fall
// back to its own section-specific bindings.
func (r *results) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if r.detailing {
		return r.updateDetail(msg), true
	}
	if r.ctrl.Active() {
		return r.updateAction(msg), true
	}
	if r.confirm != nil {
		return r.updateConfirm(msg), true
	}
	if r.jumping {
		return r.updateJump(msg), true
	}
	if r.faceting {
		return r.updateFacet(msg), true
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
			r.applyFilter()
		}
	case key.Matches(msg, k.SelectAll):
		r.selectAllShown()
		r.applyFilter()
	case key.Matches(msg, k.SelectInvert):
		r.invertShown()
		r.applyFilter()
	case key.Matches(msg, k.SelectRange):
		r.selectRange()
		r.applyFilter()
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
			r.ctrl.OpenText(action.ModeBulkComment, "", "")
			return nil, true
		}
		return r.openComment(), true
	case key.Matches(msg, k.Edit):
		if iss := r.selected(); iss != nil {
			r.openTextVerb(action.ModeEdit, issueSummary(iss))
		}
	case key.Matches(msg, k.Assign):
		if len(r.markedKeys()) > 0 && r.canMutate() {
			r.ctrl.OpenText(action.ModeBulkAssign, "", "")
			return nil, true
		}
		r.openTextVerb(action.ModeAssign, "")
	case key.Matches(msg, k.Worklog):
		r.openTextVerb(action.ModeWorklog, "")
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

// autoRefresh refetches on the shared refresh tick. It deliberately skips when
// the user is mid-interaction (filter/modal/detail), a fetch is already in
// flight, or a mutation is reconciling — a background reload must never stomp
// optimistic state or yank the view out from under the user.
func (r *results) autoRefresh() tea.Cmd {
	// loadingMore counts as in-flight too: a tick mid-pagination would
	// supersede the page fetch and yank the user back to page one.
	if r.refetch == nil || r.capturing() || r.loading || r.loadingMore || !r.canMutate() {
		return nil
	}
	return r.refetch()
}

// canMutate reports that no single transition or bulk batch is mid-flight, so a
// new mutation won't race the optimistic rollback or a pending reconcile.
func (r *results) canMutate() bool { return r.rollback == nil && !r.bulkPending && !r.writing }

// openTextVerb opens a free-text action against the selected issue, if any and
// if no other mutation is in flight.
func (r *results) openTextVerb(mode action.Mode, initial string) {
	if iss := r.selected(); iss != nil && r.canMutate() {
		r.ctrl.OpenText(mode, issueKey(iss), initial)
	}
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
	case r.faceting:
		return r.facetPick.Update(msg), true
	case r.jumping:
		return r.jumpPick.Update(msg), true
	case r.ctrl.Active():
		return r.ctrl.Update(msg), true
	case r.filtering:
		return r.syncFilter(msg), true
	}
	return nil, false
}

// updateAction drives the open action: esc cancels; enter submits except in
// the multiline comment area, where it inserts a newline and ctrl+s submits;
// the transition picker takes up/down; everything else flows into the shared
// text input, giving the verbs real cursor movement and paste.
func (r *results) updateAction(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		r.ctrl.Cancel()
		return nil
	case "ctrl+s":
		if r.ctrl.Multiline() {
			return r.submitAction()
		}
		return nil
	case "enter":
		if !r.ctrl.Multiline() {
			return r.submitAction()
		}
	}
	// Everything else — including up/down, which the transition picker
	// consumes itself — flows into the active control.
	return r.ctrl.Update(msg)
}

// openComment starts a comment: through the external editor when one is
// configured ($JIRA_EDITOR/$EDITOR — the TUI suspends while it runs),
// otherwise the in-modal textarea.
func (r *results) openComment() tea.Cmd {
	iss := r.selected()
	if iss == nil || !r.canMutate() {
		return nil
	}
	if input.EditorCommand() != "" {
		return input.Edit("comment:"+issueKey(iss), "")
	}
	r.ctrl.OpenText(action.ModeComment, issueKey(iss), "")
	return nil
}

// handleEditor resumes a flow that went through the external editor: an error
// or empty buffer surfaces and nothing is written; otherwise the text submits
// exactly like its in-modal equivalent.
func (r *results) handleEditor(msg input.EditorFinishedMsg) tea.Cmd {
	if msg.Err != nil {
		r.err = msg.Err
		return nil
	}
	kind, issKey, ok := strings.Cut(msg.ID, ":")
	if !ok || kind != "comment" {
		return nil
	}
	if strings.TrimSpace(msg.Text) == "" {
		return r.flashNotice("empty comment discarded", false)
	}
	body, _, err := adf.FromMarkdownLossy(msg.Text)
	if err != nil {
		r.err = err
		return nil
	}
	return r.mutate(func(svc core.Services, base context.Context) error {
		_, _, e := svc.Issues().AddComment(base, issKey, &jira.CommentAddRequest{Body: body})
		return e
	})
}

func (r *results) fetchTransitions(key string, bulk bool) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.transitionsScope(),
		Run: func() (any, error) {
			if svc == nil {
				return transitionsResult{issueKey: key, bulk: bulk}, nil
			}
			ts, _, err := svc.Issues().Transitions(base, key)
			if err != nil {
				return nil, err
			}
			return transitionsResult{issueKey: key, transitions: ts, bulk: bulk}, nil
		},
	})
}

func (r *results) submitAction() tea.Cmd {
	req, ok := r.ctrl.Submit()
	if !ok {
		return nil
	}
	switch req.Mode {
	case action.ModeTransition:
		r.rollback = r.applyOptimisticTransition(req.IssueKey, req.TransitionName)
		base := r.ctx.Base
		svc := r.ctx.Services
		key, id := req.IssueKey, req.TransitionID
		return r.ctx.StartTask(core.TaskSpec{
			Scope: r.mutateScope(),
			Run: func() (any, error) {
				if svc == nil {
					return nil, nil
				}
				_, err := svc.Issues().Transition(base, key, &jira.TransitionRequest{ID: id})
				return nil, err
			},
		})
	case action.ModeBulkTransition, action.ModeBulkAssign, action.ModeBulkComment:
		// Bulk writes hit every marked issue at once, so they park behind a
		// y/N confirmation (default No) instead of running off the submit.
		keys := r.markedKeys()
		if len(keys) == 0 {
			return nil
		}
		text := req.Text
		switch req.Mode {
		case action.ModeBulkTransition:
			// The picker carries the chosen name verbatim — no trim, because
			// the apply side matches it exactly against each issue's
			// transition names, whitespace included; per-issue ids resolve at
			// apply time because each issue exposes its own transitions.
			text = req.TransitionName
		case action.ModeBulkAssign:
			// Assignee queries are lookup keys; comments post verbatim
			// (leading whitespace is markdown).
			text = strings.TrimSpace(text)
		}
		r.confirm = &bulkConfirm{mode: req.Mode, text: text, keys: keys}
		return nil
	case action.ModeComment:
		body, _, err := adf.FromMarkdownLossy(req.Text)
		if err != nil {
			r.err = err
			return nil
		}
		issKey := req.IssueKey
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().AddComment(base, issKey, &jira.CommentAddRequest{Body: body})
			return e
		})
	case action.ModeEdit:
		key, summary := req.IssueKey, req.Text
		r.rollback = r.applyOptimisticSummary(key, summary)
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().Update(base, key, &jira.IssueUpdateRequest{
				Fields: map[string]any{"summary": summary},
			})
			return e
		})
	case action.ModeAssign:
		// One normalised value drives both the optimistic row and the write:
		// "" means unassign for each.
		display := strings.TrimSpace(req.Text)
		if clearsAssignee(display) {
			display = ""
		}
		r.rollback = r.applyOptimisticAssignee(req.IssueKey, display)
		return r.assignTo(req.IssueKey, display)
	case action.ModeWorklog:
		wd := r.ctx.WorkdaySeconds
		if wd <= 0 {
			wd = workdaySeconds
		}
		secs, err := jira.ParseDuration(req.Text, wd)
		if err != nil {
			r.err = err
			return nil
		}
		key := req.IssueKey
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Worklogs().Add(base, key, &jira.WorklogAddRequest{TimeSpentSeconds: secs})
			return e
		})
	}
	return nil
}

// mutate runs a single-issue write on the mutate scope. On success handleTask
// reconciles by refetching; on error it rolls back any optimistic change the
// caller registered (r.rollback) and shows the failure toast. Verbs whose
// field shows in the row (edit, assign) apply optimistically before calling
// this; comment and worklog have nothing row-visible, so they just reconcile.
func (r *results) mutate(run func(svc core.Services, base context.Context) error) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	r.writing = true // block overlapping writes until this one reconciles
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.mutateScope(),
		Run: func() (any, error) {
			if svc == nil {
				return nil, nil
			}
			return nil, run(svc, base)
		},
	})
}

// assignMe resolves the current user's account id and assigns the issue to them.
func (r *results) assignMe(key string) tea.Cmd {
	return r.mutate(func(svc core.Services, base context.Context) error {
		me, _, err := svc.Users().Myself(base)
		if err != nil {
			return err
		}
		if me.AccountID == "" {
			return fmt.Errorf("could not resolve your account id for self-assign")
		}
		return setAssignee(svc, base, key, me.AccountID)
	})
}

// assignTo resolves a user query (name/email) to an account id and assigns it.
// The keywords "none"/"unassigned" (or an empty query) clear the assignee
// instead of being treated as a user search.
func (r *results) assignTo(key, query string) tea.Cmd {
	q := strings.TrimSpace(query)
	return r.mutate(func(svc core.Services, base context.Context) error {
		if q == "" || strings.EqualFold(q, "none") || strings.EqualFold(q, "unassigned") {
			return setAssignee(svc, base, key, "")
		}
		id, err := svc.Users().ResolveUser(base, q)
		if err != nil {
			return err
		}
		return setAssignee(svc, base, key, id)
	})
}

// setAssignee writes the assignee field; an empty id clears it (unassigned).
func setAssignee(svc core.Services, base context.Context, key, accountID string) error {
	var assignee any
	if accountID != "" {
		assignee = map[string]any{"accountId": accountID}
	}
	_, _, err := svc.Issues().Update(base, key, &jira.IssueUpdateRequest{
		Fields: map[string]any{"assignee": assignee},
	})
	return err
}

// openInBrowser opens the issue's web page. The open is fire-and-forget: a
// failure to launch a browser shouldn't disrupt the dashboard.
func (r *results) openInBrowser(key string) tea.Cmd {
	url := r.issueURL(key)
	if url == "" {
		return nil
	}
	base := r.ctx.Base
	return func() tea.Msg {
		_ = browser.Open(base, url)
		return nil
	}
}

// issueURL builds the browse URL for an issue via the browser helper (which
// trims the base and path-escapes the key), or "" if no base URL is known.
func (r *results) issueURL(key string) string {
	if xstrings.AnyEmpty(r.ctx.BaseURL, key) {
		return ""
	}
	return browser.IssueURL(r.ctx.BaseURL, key)
}

// transitionByName resolves the transition whose name matches status for one
// issue and applies it. Match is by transition name (jira.Transition models only
// id+name, not the destination status), which for default Jira workflows equals
// the target status. The available transitions are issue-specific, so a name
// that isn't reachable for this issue surfaces as a per-issue error.
func transitionByName(ctx context.Context, svc core.Services, key, status string) error {
	ts, _, err := svc.Issues().Transitions(ctx, key)
	if err != nil {
		return fmt.Errorf("list transitions: %w", err)
	}
	id := findTransitionID(ts, status)
	if id == "" {
		return fmt.Errorf("no transition to %q", status)
	}
	if _, err := svc.Issues().Transition(ctx, key, &jira.TransitionRequest{ID: id}); err != nil {
		return fmt.Errorf("apply transition: %w", err)
	}
	return nil
}

// findTransitionID returns the id of the transition named status (case-folded),
// or "" when none matches.
func findTransitionID(ts []*jira.Transition, status string) string {
	for _, t := range ts {
		if strings.EqualFold(ptr.Deref(t.Name), status) {
			return ptr.Deref(t.ID)
		}
	}
	return ""
}

// optimistic applies a local field change to the issue immediately (so the row
// reflects the write before the server confirms) and returns the rollback that
// restores it. change mutates the fields and returns its own undo; optimistic
// wraps both sides with the row rebuild.
func (r *results) optimistic(key string, change func(f *jira.IssueFields) func()) func() {
	iss := r.find(key)
	if iss == nil || iss.Fields == nil {
		return func() {}
	}
	undo := change(iss.Fields)
	r.applyFilter()
	return func() {
		undo()
		r.applyFilter()
	}
}

func (r *results) applyOptimisticTransition(key, newStatus string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		if f.Status == nil {
			f.Status = &jira.Status{}
		}
		prev := f.Status.Name
		ns := newStatus
		f.Status.Name = &ns
		return func() { f.Status.Name = prev }
	})
}

// applyOptimisticSummary swaps the row summary for an edit in flight.
func (r *results) applyOptimisticSummary(key, summary string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		prev := f.Summary
		s := summary
		f.Summary = &s
		return func() { f.Summary = prev }
	})
}

// applyOptimisticAssignee shows the assignee column as the typed query (or
// clears it for an unassign) while the write runs. The server's canonical
// display name replaces it when the post-write reconcile lands.
func (r *results) applyOptimisticAssignee(key, display string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		prev := f.Assignee
		if display == "" {
			f.Assignee = nil
		} else {
			d := display
			f.Assignee = &jira.User{DisplayName: &d}
		}
		return func() { f.Assignee = prev }
	})
}

// view composes the header (owner-supplied), status line, list, and sidebar,
// overlaying the action controller when open.
func (r *results) view(header string) string {
	if r.detailing {
		hint := r.ctx.Styles.Footer.Render("esc/enter back · tab switch · ↑/↓ scroll · o open in browser · q quit")
		return lipgloss.JoinVertical(lipgloss.Left, header, r.detailPills(), hint, r.detailBody())
	}
	main := lipgloss.JoinVertical(lipgloss.Left, header, r.statusLine(), columnHeader(r.ctx.MainWidth-3), r.list.View())

	body := main
	if r.ctx.SidebarOpen && r.ctx.PreviewWidth > 0 {
		side := r.ctx.Styles.Sidebar.Render(vpWithBar(r.preview))
		switch r.ctx.PreviewPosition() {
		case core.PreviewRight:
			side = r.ctx.Styles.SidebarBorder.Render(side)
			body = lipgloss.JoinHorizontal(lipgloss.Top, main, side)
		case core.PreviewLeft:
			side = r.ctx.Styles.SidebarBorderRight.Render(side)
			body = lipgloss.JoinHorizontal(lipgloss.Top, side, main)
		default: // bottom
			side = r.ctx.Styles.SidebarBorderTop.Render(side)
			body = lipgloss.JoinVertical(lipgloss.Left, main, side)
		}
	}
	if content := r.overlayContent(); content != "" {
		// Cap the modal width and wrap its content so a long edit prefill (e.g.
		// the full summary) can never bleed past the screen edges.
		boxW := r.ctx.ScreenWidth - 6
		if boxW > 66 {
			boxW = 66
		}
		if boxW < 1 {
			boxW = 1
		}
		box := r.ctx.Styles.Overlay.Width(boxW).Render(content)
		body = overlay.Place(body, box, r.ctx.ScreenWidth, r.ctx.MainHeight, overlay.Center)
	}
	return body
}

// overlayContent is whichever modal is open: an action in flight, the facet
// or jumplist picker, or a bulk confirmation. Empty when none.
func (r *results) overlayContent() string {
	switch {
	case r.ctrl.Active():
		return r.ctrl.View()
	case r.confirm != nil:
		return r.confirm.prompt()
	case r.faceting:
		return r.facetPick.View()
	case r.jumping:
		return r.jumpPick.View()
	default:
		return ""
	}
}

func (r *results) statusLine() string {
	switch {
	case r.filtering:
		return r.filterInput.View()
	case r.flash.Active():
		// Above the sticky fetch error: a toast is visible for 4 seconds and
		// the error comes straight back; the other order hides the toast
		// entirely behind an error the user has already seen.
		if r.flash.Err {
			return theme.StatusErr.Render(r.flash.Msg)
		}
		return r.flash.Msg
	case r.err != nil:
		return r.ctx.Styles.Error.Render("error: " + r.err.Error())
	case r.loading:
		// Above the facet chip: a refresh in flight must stay visible even
		// while a facet narrows the rows.
		return r.spin.View() + " loading…"
	case r.facet.active():
		chip := r.facet.String()
		if r.filter != "" {
			chip += " · filter: " + r.filter
		}
		return fmt.Sprintf("%s · %d shown · f to change", chip, len(r.shown))
	case len(r.marks) > 0:
		return fmt.Sprintf("%d selected · t/a/c act on all", len(r.marks))
	case r.filter != "":
		return "filter: " + r.filter
	case r.loadingMore:
		return fmt.Sprintf("%d loaded · %s fetching more…", len(r.all), r.spin.View())
	case r.fetched && r.cursor.More():
		return fmt.Sprintf("%d loaded · ↓ to bottom for more", len(r.all))
	default:
		return ""
	}
}
