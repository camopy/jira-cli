package issues

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

type fakeUserSvc struct {
	jira.UserService
	accountID string
	resolved  string
}

func (f fakeUserSvc) Myself(context.Context) (*jira.CurrentUser, *jira.Response, error) {
	return &jira.CurrentUser{AccountID: f.accountID}, nil, nil
}

func (f fakeUserSvc) ResolveUser(context.Context, string) (string, error) {
	return f.resolved, nil
}

type fakeWorklogSvc struct {
	jira.WorklogService
	rec *callRecorder
}

func (f fakeWorklogSvc) Add(_ context.Context, key string, req *jira.WorklogAddRequest) (*jira.Worklog, *jira.Response, error) {
	if f.rec != nil && req != nil {
		f.rec.record("worklog:"+key, strconv.Itoa(req.TimeSpentSeconds))
	}
	return nil, nil, nil
}

// runVerb dispatches a completed action request and applies its task result —
// the section-level equivalent of the form dialog emitting formSubmitMsg.
func runVerb(t *testing.T, m *Model, req action.Request) {
	t.Helper()
	cmd, _ := m.dispatchSubmit(req)
	if cmd == nil {
		t.Fatal("dispatchSubmit returned no command")
	}
	m.Update(cmd())
}

func newVerbModel(t *testing.T, svc fakeServices) *Model {
	t.Helper()
	ctx := newTestCtx(svc)
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "old summary")}
	m.applyFilter()
	return m
}

func TestEditSummaryUpdatesIssue(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	runVerb(t, m, action.Request{Mode: action.ModeEdit, IssueKey: "JCT-1", Text: "new summary"})
	if got := w.get("summary:JCT-1"); got != "new summary" {
		t.Fatalf("Update(summary) = %q, want %q", got, "new summary")
	}
}

func TestCommentPostsADF(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	runVerb(t, m, action.Request{Mode: action.ModeComment, IssueKey: "JCT-1", Text: "looks good"})
	if w.get("comment:JCT-1") != "1" {
		t.Errorf("AddComment not called; recorder=%v", w.posted)
	}
}

func TestAssignMeResolvesAccountAndAssigns(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{writes: w}, user: fakeUserSvc{accountID: "acc-123"}}
	m := newVerbModel(t, svc)
	cmd := m.assignMe("JCT-1")
	m.Update(cmd())
	if got := w.get("assignee:JCT-1"); got != "acc-123" {
		t.Errorf("assignee = %q, want acc-123 (recorder=%v)", got, w.posted)
	}
}

func TestAssignToResolvesQuery(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{writes: w}, user: fakeUserSvc{resolved: "acc-bob"}}
	m := newVerbModel(t, svc)
	runVerb(t, m, action.Request{Mode: action.ModeAssign, IssueKey: "JCT-1", Text: "bob"})
	if got := w.get("assignee:JCT-1"); got != "acc-bob" {
		t.Errorf("assignee = %q, want acc-bob", got)
	}
}

func TestWorklogParsesDurationToSeconds(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{}, worklog: fakeWorklogSvc{rec: w}}
	m := newVerbModel(t, svc)
	runVerb(t, m, action.Request{Mode: action.ModeWorklog, IssueKey: "JCT-1", Text: "2h"})
	if got := w.get("worklog:JCT-1"); got != "7200" {
		t.Errorf("worklog seconds = %q, want 7200", got)
	}
}

func TestWriteInFlightBlocksAnotherWrite(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	cmd, _ := m.dispatchSubmit(action.Request{Mode: action.ModeEdit, IssueKey: "JCT-1", Text: "y"})
	if cmd == nil {
		t.Fatal("dispatchSubmit returned no command")
	}
	if m.canMutate() {
		t.Error("a write in flight must block another mutation")
	}
	m.Update(cmd()) // reconcile completes
	if !m.canMutate() {
		t.Error("mutation guard not cleared after the write completed")
	}
}

func TestAssignNoneClearsAssignee(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	cmd := m.assignTo("JCT-1", "none")
	m.Update(cmd())
	if got := w.get("assignee:JCT-1"); got != "unassigned" {
		t.Errorf("assignee = %q, want unassigned (cleared)", got)
	}
}

func TestIssueURLUsesBaseURL(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.ctx.BaseURL = "https://acme.atlassian.net/"
	if got := m.issueURL("JCT-1"); got != "https://acme.atlassian.net/browse/JCT-1" {
		t.Errorf("issueURL = %q", got)
	}
	m.ctx.BaseURL = ""
	if got := m.issueURL("JCT-1"); got != "" {
		t.Errorf("issueURL with no base = %q, want empty", got)
	}
}

var _ core.Services = fakeServices{}

func TestLabelsReplaceWholeList(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	runVerb(t, m, action.Request{Mode: action.ModeLabels, IssueKey: "JCT-1", Text: "bug, ux,, triage "})
	if got := w.get("labels:JCT-1"); got != "bug,ux,triage" {
		t.Fatalf("Update(labels) = %q, want bug,ux,triage", got)
	}
}

func TestLabelsEmptyInputClearsAll(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	// An emptied labels field submits as "remove every label".
	runVerb(t, m, action.Request{Mode: action.ModeLabels, IssueKey: "JCT-1", Text: ""})
	if got, ok := w.posted["labels:JCT-1"]; !ok || got != "" {
		t.Fatalf("Update(labels) = %q (recorded=%v), want recorded empty list", got, ok)
	}
}

func TestCreateOverlayCollectsSummaryAndDescription(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	runVerb(t, m, action.Request{Mode: action.ModeCreate, IssueKey: "JCT", Summary: "Fix the flux", Text: "It sparks when engaged."})
	if got := w.get("create:JCT"); got != "Fix the flux|Task|with-description" {
		t.Fatalf("Create = %q, want summary|Task|description", got)
	}
}

// A single transition records a footer/log entry like every other write — it
// runs its own task rather than r.mutate, so the recording is easy to drop.
func TestSingleTransitionRecordsActivity(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.applySingleTransition("JCT-1", "21", "In Progress")
	e, ok := m.ctx.Activity.Recent()
	if !ok || !strings.Contains(e.Pending, "JCT-1") {
		t.Fatalf("transition recorded no pending entry naming JCT-1: %+v (ok=%v)", e, ok)
	}
	// On resolution the key lands so the footer/log can hyperlink it.
	m.finishOp(nil)
	if e, _ = m.ctx.Activity.Recent(); e.IssueKey != "JCT-1" {
		t.Fatalf("resolved transition entry = %+v, want IssueKey JCT-1", e)
	}
}

func TestCreateCarriesTypeAssigneeAndLabels(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctx.DefaultIssueType = "Bug" // the request's own type must win over this
	runVerb(t, m, action.Request{
		Mode:      action.ModeCreate,
		IssueKey:  "JCT",
		Summary:   "Ship the widget",
		IssueType: "Story",
		Assignee:  "acc-42",
		Labels:    []string{"ux", "backend"},
	})
	if got := w.get("create:JCT"); got != "Ship the widget|Story|" {
		t.Fatalf("create payload = %q, want type Story with no description", got)
	}
	if got := w.get("create-assignee:JCT"); got != "acc-42" {
		t.Fatalf("create assignee = %q, want the accepted accountId acc-42", got)
	}
	if got := w.get("create-labels:JCT"); got != "ux,backend" {
		t.Fatalf("create labels = %q, want ux,backend", got)
	}
}

func TestCreateWithoutSummaryStaysOpen(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.openCreateForm("JCT", nil, nil)
	m.passGrace() // the async-open grace must not be what blocks the submit
	// ctrl+s on a blank required summary: the form blocks the submit (no
	// formSubmitMsg) and stays on the stack for the user to finish.
	cmd := m.updateDialog(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("submit with an empty summary must not emit a command")
	}
	if m.activeForm() == nil {
		t.Fatal("the create overlay must stay open for the user to finish")
	}
}

func TestCreateUsesProfileIssueType(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctx.DefaultIssueType = "Bug"
	runVerb(t, m, action.Request{Mode: action.ModeCreate, IssueKey: "JCT", Summary: "Only a summary"})
	if got := w.get("create:JCT"); got != "Only a summary|Bug|" {
		t.Fatalf("Create = %q, want summary-only with profile type", got)
	}
}
