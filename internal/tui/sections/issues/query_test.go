package issues

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// TestQuerySectionRunsConfiguredJQL pins the config-section contract: Init
// starts the configured fetch, the tab carries the configured title, and the
// landed result feeds the tab count.
func TestQuerySectionRunsConfiguredJQL(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	s := NewQuery(QueryID(0), "Team Board", "project = JCT")(ctx)
	if s.Title() != "Team Board" {
		t.Errorf("Title = %q, want Team Board", s.Title())
	}
	if cmd := s.Init(ctx); cmd == nil {
		t.Fatal("Init should start the configured fetch")
	}
	q := s.(*QueryModel)
	q.Update(core.TaskFinishedMsg{
		Scope:  q.fetchScope(),
		Result: fetchResult{issues: []*jira.Issue{mkIssue("JCT-1", "To Do", "one")}},
	})
	if n, ok := q.Count(); !ok || n != 1 {
		t.Errorf("Count = %d,%v; want 1,true", n, ok)
	}
}

// TestAutoRefreshSkipsWhileBusy pins the tick gating: no refetch while a fetch
// is in flight or the user is typing; an idle section refetches.
func TestAutoRefreshSkipsWhileBusy(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	m := New(ctx).(*Model)
	m.Init(ctx) // fetch now in flight

	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick during an in-flight fetch should be a no-op")
	}
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{}})
	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd == nil {
		t.Error("an idle tick should refetch")
	}
	m.filtering = true
	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick while typing a filter should be a no-op")
	}
}

// TestRefetchPreservesSelectionByKey pins the background-refresh safety: when a
// refetch reorders the results, the cursor follows the issue, not the index —
// otherwise an auto-refresh could swap the issue under a pending action.
func TestRefetchPreservesSelectionByKey(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{issues: []*jira.Issue{
		mkIssue("JCT-1", "To Do", "one"),
		mkIssue("JCT-2", "To Do", "two"),
		mkIssue("JCT-3", "To Do", "three"),
	}}})
	m.list.SetCursor(1) // JCT-2

	// A refresh lands with JCT-2 moved to the top.
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{issues: []*jira.Issue{
		mkIssue("JCT-2", "To Do", "two"),
		mkIssue("JCT-1", "To Do", "one"),
		mkIssue("JCT-3", "To Do", "three"),
	}}})
	sel := m.selected()
	if sel == nil || issueKey(sel) != "JCT-2" {
		t.Fatalf("selection after reordering refetch = %v, want JCT-2", sel)
	}

	// The selected issue dropping out leaves a valid clamped cursor.
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{issues: []*jira.Issue{
		mkIssue("JCT-9", "To Do", "nine"),
	}}})
	if sel := m.selected(); sel == nil {
		t.Fatal("selection should clamp to a valid row when the old issue is gone")
	}
}

// TestFetchMorePagination pins the scroll-to-fetch contract: landing on the
// last loaded row with a continuation token pulls the next page, the page is
// appended without moving the cursor, and the final page stops the cycle.
func TestFetchMorePagination(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{
		issues: []*jira.Issue{
			mkIssue("JCT-1", "To Do", "one"),
			mkIssue("JCT-2", "To Do", "two"),
		},
		cursor: jira.CursorForTest("tok-2"),
	}})

	// Mid-list: no fetch-more.
	if cmd := m.maybeFetchMore(); cmd != nil {
		t.Error("fetch-more should not fire while the cursor is mid-list")
	}
	m.list.Bottom()
	cmd := m.maybeFetchMore()
	if cmd == nil {
		t.Fatal("cursor on the last row with a token should fetch the next page")
	}
	if m.maybeFetchMore() != nil {
		t.Error("a second trigger while the page fetch is in flight should be a no-op")
	}

	cur := m.list.Cursor()
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchMoreResult{
		issues: []*jira.Issue{mkIssue("JCT-3", "To Do", "three")},
	}})
	if len(m.all) != 3 {
		t.Fatalf("after fetch-more, %d issues loaded; want 3", len(m.all))
	}
	if m.list.Cursor() != cur {
		t.Errorf("appending a page moved the cursor (%d → %d)", cur, m.list.Cursor())
	}
	m.list.Bottom()
	if m.maybeFetchMore() != nil {
		t.Error("the final page must stop the fetch-more cycle")
	}
}

// TestFetchMoreErrorUnblocksPagination pins the failure path: a failed page
// fetch must clear the in-flight flag so the user can retry by scrolling, and
// an auto-refresh tick must not fire mid-pagination.
func TestFetchMoreErrorUnblocksPagination(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{
		issues: []*jira.Issue{mkIssue("JCT-1", "To Do", "one")},
		cursor: jira.CursorForTest("tok-2"),
	}})
	m.list.Bottom()
	if m.maybeFetchMore() == nil {
		t.Fatal("setup: fetch-more should have started")
	}

	// Mid-pagination, the refresh tick is a no-op.
	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick during a page fetch should be a no-op")
	}

	// The page fetch fails: pagination must unblock for a retry.
	m.Update(core.TaskFinishedMsg{Scope: m.fetchScope(), Err: errBoom})
	if m.loadingMore {
		t.Error("a failed page fetch must clear loadingMore")
	}
	if m.maybeFetchMore() == nil {
		t.Error("scrolling at the bottom should retry after a failed page fetch")
	}
}
