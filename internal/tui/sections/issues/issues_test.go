package issues

import (
	"context"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// callRecorder captures the transition id posted per issue so bulk tests can
// assert each issue gets its own id. It is concurrency-safe because the bulk
// path applies transitions from a worker pool. It is a pointer field on the
// fake so copying the fake (by value) never copies the mutex.
type callRecorder struct {
	mu     sync.Mutex
	posted map[string]string
}

func (c *callRecorder) record(key, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.posted == nil {
		c.posted = make(map[string]string)
	}
	c.posted[key] = id
}

func (c *callRecorder) get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.posted[key]
}

// fakeIssueSvc overrides the methods the section uses; embedding the interface
// satisfies the rest.
type fakeIssueSvc struct {
	jira.IssueService
	issues        []*jira.Issue
	err           error
	transitions   []*jira.Transition
	transitionErr error

	// transitionsByKey overrides transitions per issue (bulk tests); transErrByKey
	// makes a specific issue's Transition call fail.
	transitionsByKey map[string][]*jira.Transition
	transErrByKey    map[string]error
	rec              *callRecorder
	// writes records Update/AddComment calls for verb tests (keyed by verb).
	writes *callRecorder
	// updateErr makes Update fail, for rollback tests.
	updateErr error
	// full is returned by Get for the detail view.
	full *jira.Issue
}

func (f fakeIssueSvc) Get(context.Context, string, *jira.IssueGetOptions) (*jira.Issue, *jira.Response, error) {
	return f.full, nil, nil
}

func (f fakeIssueSvc) Update(_ context.Context, key string, req *jira.IssueUpdateRequest) (*jira.Issue, *jira.Response, error) {
	if f.writes != nil && req != nil {
		if v, ok := req.Fields["summary"]; ok {
			f.writes.record("summary:"+key, v.(string))
		}
		if a, ok := req.Fields["assignee"]; ok {
			id := "unassigned"
			if m, ok := a.(map[string]any); ok {
				id, _ = m["accountId"].(string)
			}
			f.writes.record("assignee:"+key, id)
		}
	}
	return nil, nil, f.updateErr
}

func (f fakeIssueSvc) AddComment(_ context.Context, key string, _ *jira.CommentAddRequest) (*jira.Comment, *jira.Response, error) {
	if f.writes != nil {
		f.writes.record("comment:"+key, "1")
	}
	return nil, nil, nil
}

func (f fakeIssueSvc) List(context.Context, *jira.IssueListOptions) ([]*jira.Issue, *jira.Response, error) {
	return f.issues, nil, f.err
}

func (f fakeIssueSvc) Transitions(_ context.Context, key string) ([]*jira.Transition, *jira.Response, error) {
	if f.transitionsByKey != nil {
		return f.transitionsByKey[key], nil, nil
	}
	return f.transitions, nil, nil
}

func (f fakeIssueSvc) Transition(_ context.Context, key string, req *jira.TransitionRequest) (*jira.Response, error) {
	if f.rec != nil && req != nil {
		f.rec.record(key, req.ID)
	}
	if f.transErrByKey != nil {
		if err, ok := f.transErrByKey[key]; ok {
			return nil, err
		}
	}
	return nil, f.transitionErr
}

func mkTransition(id, name string) *jira.Transition {
	i, n := id, name
	return &jira.Transition{ID: &i, Name: &n}
}

// fakeJQLSvc overrides Parse; default returns a valid (error-free) parse.
type fakeJQLSvc struct {
	jira.JQLService
	parseErrors []string
}

func (f fakeJQLSvc) Parse(context.Context, []string, string) ([]jira.ParsedQuery, *jira.Response, error) {
	return []jira.ParsedQuery{{Errors: f.parseErrors}}, nil, nil
}

// fakeServices overrides Issues() and JQL().
type fakeServices struct {
	core.Services
	issue   jira.IssueService
	jql     jira.JQLService
	user    jira.UserService
	worklog jira.WorklogService
}

func (f fakeServices) Issues() jira.IssueService { return f.issue }

func (f fakeServices) Users() jira.UserService { return f.user }

func (f fakeServices) Worklogs() jira.WorklogService { return f.worklog }

func (f fakeServices) JQL() jira.JQLService {
	if f.jql == nil {
		return fakeJQLSvc{}
	}
	return f.jql
}

func mkIssue(key, status, summary string) *jira.Issue {
	k, st, su := key, status, summary
	return &jira.Issue{
		Key:    &k,
		Fields: &jira.IssueFields{Summary: &su, Status: &jira.Status{Name: &st}},
	}
}

func newTestCtx(svc core.Services) *core.ProgramContext {
	ctx := core.NewProgramContext(svc, nil)
	ctx.StartTask = core.NewTaskManager().Start // real task manager for fetch
	ctx.SetSize(120, 40)
	return ctx
}

func TestDefaultLensIsMyOpenWork(t *testing.T) {
	jql := DefaultLens().JQL
	for _, want := range []string{"currentUser()", "statusCategory != Done", "ORDER BY updated"} {
		if !strings.Contains(jql, want) {
			t.Errorf("triage JQL %q missing %q", jql, want)
		}
	}
}

func TestInitFetchesAndPopulatesList(t *testing.T) {
	data := []*jira.Issue{
		mkIssue("JCT-1", "To Do", "First issue"),
		mkIssue("JCT-2", "In Progress", "Second issue"),
	}
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{issues: data}})
	m := New(ctx).(*Model)

	cmd := m.Init(ctx)
	if cmd == nil {
		t.Fatal("Init returned no fetch command")
	}
	msg := taskMsg(t, cmd)
	fin, ok := msg.(core.TaskFinishedMsg)
	if !ok {
		t.Fatalf("fetch command did not produce TaskFinishedMsg, got %T", msg)
	}
	if fin.Scope != m.fetchScope() {
		t.Errorf("fetch scope = %q, want %q", fin.Scope, m.fetchScope())
	}

	m.Update(fin)
	if len(m.shown) != 2 {
		t.Fatalf("shown issues = %d, want 2", len(m.shown))
	}
	if m.list.Len() != 2 {
		t.Errorf("list rows = %d, want 2", m.list.Len())
	}
	if m.selected() == nil || issueKey(m.selected()) != "JCT-1" {
		t.Errorf("selected = %v, want JCT-1", m.selected())
	}
}

func TestLocalFilterNarrowsList(t *testing.T) {
	data := []*jira.Issue{
		mkIssue("JCT-1", "To Do", "Fix the build"),
		mkIssue("JCT-2", "In Progress", "Write docs"),
	}
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{issues: data}})
	m := New(ctx).(*Model)
	m.all = data
	m.filter = "docs"
	m.applyFilter()
	if len(m.shown) != 1 || issueKey(m.shown[0]) != "JCT-2" {
		t.Errorf("filter 'docs' shown = %v, want [JCT-2]", m.shown)
	}
}

// taskMsg runs a cmd — descending into batches (a fetch cmd carries the
// spinner tick alongside the task) — and returns the first TaskFinishedMsg.
func taskMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil cmd where a task was expected")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if m := c(); m != nil {
				if _, ok := m.(core.TaskFinishedMsg); ok {
					return m
				}
			}
		}
		t.Fatal("batch contained no TaskFinishedMsg")
	}
	return msg
}

func TestFetchErrorStoredNotPanicked(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{err: errBoom}})
	sec := New(ctx)
	fin := taskMsg(t, sec.Init(ctx))
	sec, _ = sec.Update(fin)
	m := sec.(*Model)
	if m.err == nil {
		t.Error("fetch error was not stored on the model")
	}
	if m.loading {
		t.Error("loading should be cleared after a failed fetch")
	}
}

func TestFetchTransitionsOpensPicker(t *testing.T) {
	svc := fakeIssueSvc{transitions: []*jira.Transition{mkTransition("41", "Done")}}
	ctx := newTestCtx(fakeServices{issue: svc})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	m.applyFilter()

	cmd := m.fetchTransitions("JCT-1", false)
	sec, _ := m.Update(cmd())
	m = sec.(*Model)
	if !m.ctrl.Active() {
		t.Fatal("picker not opened after transitions fetch")
	}
}

func TestTransitionOptimisticThenRollbackOnError(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{transitionErr: errBoom}})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	m.applyFilter()

	// Apply the transition optimistically.
	m.rollback = m.applyOptimisticTransition("JCT-1", "Done")
	if got := issueStatus(m.find("JCT-1")); got != "Done" {
		t.Fatalf("optimistic status = %q, want Done", got)
	}

	// The mutation fails: the change must be rolled back and the error shown.
	m.handleTask(core.TaskFinishedMsg{Scope: m.mutateScope(), Err: errBoom})
	if got := issueStatus(m.find("JCT-1")); got != "To Do" {
		t.Errorf("status after rollback = %q, want To Do", got)
	}
	if !m.flash.Active() || !m.flash.Err {
		t.Error("mutation failure should flash an error toast")
	}
}

func TestTransitionSuccessClearsRollbackAndReconciles(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	m.applyFilter()
	m.rollback = m.applyOptimisticTransition("JCT-1", "Done")

	cmd, _ := m.handleTask(core.TaskFinishedMsg{Scope: m.mutateScope()})
	if m.rollback != nil {
		t.Error("rollback not cleared on success")
	}
	if cmd == nil {
		t.Error("successful mutation should trigger a reconcile fetch")
	}
}

func TestEmptyTransitionsShowsNoticeNotPicker(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{transitions: nil}})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "Done", "x")}
	m.applyFilter()

	cmd := m.fetchTransitions("JCT-1", false)
	m.Update(cmd())
	if m.ctrl.Active() {
		t.Error("picker opened with no valid transitions")
	}
	if m.flash.Msg != "no transitions available" {
		t.Errorf("flash = %q, want 'no transitions available'", m.flash.Msg)
	}
}

func TestViewRendersControllerOverlay(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.Init(ctx) // sizes the list
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "x")}
	m.applyFilter()
	m.ctrl.OpenTransition("JCT-1", []*jira.Transition{mkTransition("41", "Done")})

	out := m.View()
	if !strings.Contains(out, "Done") {
		t.Error("active controller not rendered into the view")
	}
}

func TestSidebarShowsSelectedIssueDetail(t *testing.T) {
	iss := mkIssue("JCT-9", "Blocked", "Investigate flake")
	out := sidebar(iss, 40, mdr(), "")
	for _, want := range []string{"JCT-9", "Investigate flake", "Blocked"} {
		if !strings.Contains(out, want) {
			t.Errorf("sidebar missing %q in:\n%s", want, out)
		}
	}
}

var errBoom = boomErr("boom")

type boomErr string

func (e boomErr) Error() string { return string(e) }

func TestFlashClearsOnTickAndIgnoresStaleClear(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)

	// Build the stale clear directly (executing the flashNotice cmd would
	// sleep out the real 4s tick).
	stale := flashClearMsg{id: m.id, clear: m.flash.Set("copied JCT-1", false)}
	if !m.flash.Active() || m.flash.Msg != "copied JCT-1" {
		t.Fatalf("flash not set: %+v", m.flash)
	}
	m.flashNotice("second toast", false)

	// The stale clear must not wipe the newer toast.
	m.Update(stale)
	if m.flash.Msg != "second toast" {
		t.Errorf("stale clear wiped a newer flash: %q", m.flash.Msg)
	}

	// A fresh clear for the current toast clears it…
	current := flashClearMsg{id: m.id, clear: m.flash.Set("third", false)}
	m.Update(current)
	if m.flash.Active() {
		t.Error("matching clear did not expire the toast")
	}

	// …and a clear addressed to another section is ignored entirely, even
	// with a matching internal counter.
	foreign := flashClearMsg{id: "other-section", clear: m.flash.Set("mine", false)}
	m.Update(foreign)
	if !m.flash.Active() || m.flash.Msg != "mine" {
		t.Errorf("a foreign section's clear wiped this section's toast: %+v", m.flash)
	}
}

func TestSpinnerTicksWhileLoadingAndStopsWhenIdle(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)

	m.loading = true
	if cmd := m.handleSpinner(spinner.TickMsg{ID: m.spin.ID()}); cmd == nil {
		t.Error("spinner should keep ticking while loading")
	}
	m.loading = false
	if cmd := m.handleSpinner(spinner.TickMsg{ID: m.spin.ID()}); cmd != nil {
		t.Error("spinner should stop once the section is idle")
	}
}

func TestStatusLineShowsSpinnerWhileLoading(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.loading = true
	if line := m.statusLine(); !strings.Contains(line, "loading") {
		t.Errorf("status line = %q, want loading indicator", line)
	}
	m.loading = false
	m.flashNotice("✗ boom", true)
	if line := m.statusLine(); !strings.Contains(line, "boom") {
		t.Errorf("status line = %q, want flash toast", line)
	}
}
