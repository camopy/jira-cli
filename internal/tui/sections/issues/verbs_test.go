package issues

import (
	"context"
	"strconv"
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

// drive a completed action through submitAction and apply its task result.
func runVerb(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.submitAction()
	if cmd == nil {
		t.Fatal("submitAction returned no command")
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
	m.ctrl.OpenText(action.ModeEdit, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "new summary"})
	runVerb(t, m)
	if got := w.get("summary:JCT-1"); got != "new summary" {
		t.Fatalf("Update(summary) = %q, want %q", got, "new summary")
	}
}

func TestCommentPostsADF(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctrl.OpenText(action.ModeComment, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "looks good"})
	runVerb(t, m)
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
	m.ctrl.OpenText(action.ModeAssign, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "bob"})
	runVerb(t, m)
	if got := w.get("assignee:JCT-1"); got != "acc-bob" {
		t.Errorf("assignee = %q, want acc-bob", got)
	}
}

func TestWorklogParsesDurationToSeconds(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{}, worklog: fakeWorklogSvc{rec: w}}
	m := newVerbModel(t, svc)
	m.ctrl.OpenText(action.ModeWorklog, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "2h"})
	runVerb(t, m)
	if got := w.get("worklog:JCT-1"); got != "7200" {
		t.Errorf("worklog seconds = %q, want 7200", got)
	}
}

func TestWriteInFlightBlocksAnotherWrite(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.ctrl.OpenText(action.ModeEdit, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "y"})
	cmd := m.submitAction()
	if cmd == nil {
		t.Fatal("submitAction returned no command")
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
	m.ctrl.OpenText(action.ModeLabels, "JCT-1", "")
	m.ctrl.Update(tea.KeyPressMsg{Text: "bug, ux,, triage "})
	runVerb(t, m)
	if got := w.get("labels:JCT-1"); got != "bug,ux,triage" {
		t.Fatalf("Update(labels) = %q, want bug,ux,triage", got)
	}
}

func TestLabelsEmptyInputClearsAll(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctrl.OpenText(action.ModeLabels, "JCT-1", "stale")
	// Wipe the pre-filled value: the empty submission means "remove every label".
	for range len("stale") {
		m.ctrl.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	runVerb(t, m)
	if got, ok := w.posted["labels:JCT-1"]; !ok || got != "" {
		t.Fatalf("Update(labels) = %q (recorded=%v), want recorded empty list", got, ok)
	}
}

func TestCreateOverlayCollectsSummaryAndDescription(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctrl.OpenCreate("JCT")
	m.ctrl.Update(tea.KeyPressMsg{Text: "Fix the flux"})
	m.ctrl.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // move to the description
	m.ctrl.Update(tea.KeyPressMsg{Text: "It sparks when engaged."})
	runVerb(t, m)
	if got := w.get("create:JCT"); got != "Fix the flux|Task|with-description" {
		t.Fatalf("Create = %q, want summary|Task|description", got)
	}
}

func TestCreateWithoutSummaryStaysOpen(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.ctrl.OpenCreate("JCT")
	if cmd := m.submitAction(); cmd != nil {
		t.Fatal("submit with an empty summary must not produce a command")
	}
	if !m.ctrl.Active() {
		t.Fatal("the create overlay must stay open for the user to finish")
	}
}

func TestCreateUsesProfileIssueType(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctx.DefaultIssueType = "Bug"
	m.ctrl.OpenCreate("JCT")
	m.ctrl.Update(tea.KeyPressMsg{Text: "Only a summary"})
	runVerb(t, m)
	if got := w.get("create:JCT"); got != "Only a summary|Bug|" {
		t.Fatalf("Create = %q, want summary-only with profile type", got)
	}
}
