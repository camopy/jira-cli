package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gechr/x/ptr"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
)

func rowSummary(m *Model) string { return issueSummary(m.all[0]) }

func rowAssignee(m *Model) string {
	f := m.all[0].Fields
	if f.Assignee == nil {
		return ""
	}
	return ptr.Deref(f.Assignee.DisplayName)
}

func TestEditAppliesSummaryBeforeTheWriteLands(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	cmd, _ := m.dispatchSubmit(action.Request{Mode: action.ModeEdit, IssueKey: "JCT-1", Text: "new summary"})
	if got := rowSummary(m); got != "new summary" {
		t.Fatalf("summary before reconcile = %q, want optimistic value", got)
	}
	m.Update(cmd()) // write succeeds
	if m.rollback != nil {
		t.Error("rollback not cleared after a successful write")
	}
	if got := rowSummary(m); got != "new summary" {
		t.Errorf("summary after success = %q", got)
	}
}

func TestEditRollsBackOnWriteFailure(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{updateErr: errBoom}})
	cmd, _ := m.dispatchSubmit(action.Request{Mode: action.ModeEdit, IssueKey: "JCT-1", Text: "new summary"})
	m.Update(cmd()) // write fails
	if got := rowSummary(m); got != "old summary" {
		t.Errorf("summary after failed write = %q, want rolled back", got)
	}
	if !m.flash.Active() || !m.flash.Err {
		t.Error("failed write did not raise the error toast")
	}
}

func TestAssignShowsTypedQueryOptimistically(t *testing.T) {
	svc := fakeServices{issue: fakeIssueSvc{}, user: fakeUserSvc{resolved: "acc-bob"}}
	m := newVerbModel(t, svc)
	_, _ = m.dispatchSubmit(action.Request{Mode: action.ModeAssign, IssueKey: "JCT-1", Text: "bob"})
	if got := rowAssignee(m); got != "bob" {
		t.Errorf("assignee before reconcile = %q, want typed query", got)
	}
}

func TestAssignNoneClearsAssigneeOptimistically(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	name := "Ann"
	m.all[0].Fields.Assignee = &jira.User{DisplayName: &name}
	_, _ = m.dispatchSubmit(action.Request{Mode: action.ModeAssign, IssueKey: "JCT-1", Text: "none"})
	if got := rowAssignee(m); got != "" {
		t.Errorf("assignee after unassign = %q, want cleared", got)
	}
}

func TestAssignRollsBackOnFailure(t *testing.T) {
	svc := fakeServices{issue: fakeIssueSvc{updateErr: errBoom}, user: fakeUserSvc{resolved: "acc-bob"}}
	m := newVerbModel(t, svc)
	if got := rowAssignee(m); got != "" {
		t.Fatalf("precondition: fixture assignee = %q, want none", got)
	}
	cmd, _ := m.dispatchSubmit(action.Request{Mode: action.ModeAssign, IssueKey: "JCT-1", Text: "bob"})
	m.Update(cmd())
	if got := rowAssignee(m); got != "" {
		t.Errorf("assignee after failed write = %q, want original (none)", got)
	}
}

func TestAssignMeRollsBackOnFailure(t *testing.T) {
	svc := fakeServices{issue: fakeIssueSvc{updateErr: errBoom}, user: fakeUserSvc{accountID: "acc-123"}}
	m := newVerbModel(t, svc)
	m.Init(m.ctx)
	if got := rowAssignee(m); got != "" {
		t.Fatalf("precondition: fixture assignee = %q, want none", got)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	m.Update(cmd())
	if got := rowAssignee(m); got != "" {
		t.Errorf("assignee after failed assign-me = %q, want rolled back", got)
	}
	if !m.flash.Active() || !m.flash.Err {
		t.Error("failed assign-me did not raise the error toast")
	}
}

func TestAssignMeShowsPlaceholderOptimistically(t *testing.T) {
	svc := fakeServices{issue: fakeIssueSvc{}, user: fakeUserSvc{accountID: "acc-123"}}
	m := newVerbModel(t, svc)
	m.Init(m.ctx)
	m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	if got := rowAssignee(m); got != "me" {
		t.Errorf("assignee after A = %q, want 'me' placeholder", got)
	}
}
