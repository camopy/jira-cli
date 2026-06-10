package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func TestSearchOpensInBrowseModeThenEntersEditOnEnter(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	if cmd := s.Init(ctx); cmd != nil {
		t.Error("search should not auto-run a query on init")
	}
	// Browse mode on open: not capturing input, so the App's tab/global keys
	// work and the user can never get trapped in the JQL editor.
	if s.editing || s.CapturesInput() {
		t.Fatal("search should open in browse mode, not capturing input")
	}
	// Enter starts editing.
	if _, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); !s.editing || !s.CapturesInput() {
		t.Error("enter should start JQL editing and capture input")
	}
}

func TestSearchCommitsAndRunsQuery(t *testing.T) {
	data := []*jira.Issue{mkIssue("JCT-7", "To Do", "found")}
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{issues: data}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)

	// Open the editor (seeds the input), type a query, commit it.
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.updateEdit(tea.KeyPressMsg{Text: "project = JCT"})
	_, cmd := s.updateEdit(tea.KeyPressMsg{Code: tea.KeyEnter, Text: ""})
	if s.editing {
		t.Error("enter should leave edit mode")
	}
	if s.jql != "project = JCT" {
		t.Errorf("committed jql = %q, want 'project = JCT'", s.jql)
	}
	if cmd == nil {
		t.Fatal("committing a non-empty query should run it")
	}
	sec, _ := s.Update(taskMsg(t, cmd))
	s = sec.(*SearchModel)
	if len(s.shown) != 1 || issueKey(s.shown[0]) != "JCT-7" {
		t.Errorf("results = %v, want [JCT-7]", s.shown)
	}
}

func TestSearchInvalidJQLSurfacesErrorWithoutResults(t *testing.T) {
	svc := fakeServices{
		issue: fakeIssueSvc{issues: []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}},
		jql:   fakeJQLSvc{parseErrors: []string{"Field 'nope' does not exist"}},
	}
	ctx := newTestCtx(svc)
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.updateEdit(tea.KeyPressMsg{Text: "nope = 1"})
	_, cmd := s.updateEdit(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("commit should produce a validate+fetch task")
	}
	sec, _ := s.Update(taskMsg(t, cmd))
	s = sec.(*SearchModel)
	if s.err == nil {
		t.Error("invalid JQL did not surface an error")
	}
	if len(s.shown) != 0 {
		t.Error("invalid JQL must not populate results")
	}
}

func TestSearchEmptyQueryClearsStaleResults(t *testing.T) {
	data := []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{issues: data}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)

	// Run a query so there are results to clear.
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.updateEdit(tea.KeyPressMsg{Text: "project = JCT"})
	_, cmd := s.updateEdit(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.Update(taskMsg(t, cmd))
	if len(s.shown) != 1 {
		t.Fatalf("precondition failed: query did not populate (shown=%d)", len(s.shown))
	}

	// Committing an empty query clears the results immediately. Enter would
	// open the detail view now that a row is selected, so reopen the editor
	// with the search key and blank the seeded query.
	s.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if !s.editing {
		t.Fatal("search key did not reopen the JQL editor")
	}
	s.jqlInput.SetValue("")
	_, cmd = s.updateEdit(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(s.shown) != 0 {
		t.Error("empty commit did not clear stale results")
	}
	// The invalidation task, when it lands, must not repopulate.
	if cmd != nil {
		s.Update(taskMsg(t, cmd))
		if len(s.shown) != 0 {
			t.Error("results repopulated after an empty commit")
		}
	}
}

func TestSearchLoadsSavedQuery(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	s.editing = false

	cmd := s.loadSaved(1)
	if s.jql != s.saved()[1].JQL {
		t.Errorf("loaded jql = %q, want saved[1] %q", s.jql, s.saved()[1].JQL)
	}
	if cmd == nil {
		t.Error("loading a saved query should run it")
	}
}

func TestSearchReusesResultsActions(t *testing.T) {
	// The search section embeds results, so the optimistic transition path is
	// the same code exercised by the issues tests; here we just confirm wiring.
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	s.applyFilter()
	s.rollback = s.applyOptimisticTransition("JCT-1", "Done")
	if got := issueStatus(s.find("JCT-1")); got != "Done" {
		t.Fatalf("optimistic status = %q, want Done", got)
	}
	s.handleTask(core.TaskFinishedMsg{Scope: s.mutateScope(), Err: errBoom})
	if got := issueStatus(s.find("JCT-1")); got != "To Do" {
		t.Errorf("rollback failed: status = %q", got)
	}
}

// TestSearchTickSkipsWhileEditing pins the refresh gate: a committed query
// refetches on the tick, but never while the user is editing a new one (the
// embedded results can't see the JQL editor's focus).
func TestSearchTickSkipsWhileEditing(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)

	if _, cmd := s.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick with no committed query should be a no-op")
	}
	s.jql = "project = JCT"
	s.editing = true
	if _, cmd := s.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick while editing the JQL should be a no-op")
	}
	s.editing = false
	if _, cmd := s.Update(core.RefreshTickMsg{}); cmd == nil {
		t.Error("idle tick with a committed query should refetch")
	}
}
