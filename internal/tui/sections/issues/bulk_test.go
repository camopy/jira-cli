package issues

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// marked drives a space press, which toggles the current row's selection.
func space() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace} }

// deliver runs a command tree and feeds each resulting message back into the
// section, unwrapping batches — the harness the form dialog's async submit
// needs, since ctrl+s emits its request on a command rather than inline.
func deliver(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			deliver(t, m, c)
		}
	case nil:
	default:
		var next core.Section
		next, cmd = m.Update(msg)
		m = next.(*Model)
		deliver(t, m, cmd)
	}
}

// submitOpenForm presses ctrl+s in the open form dialog and drives the emitted
// formSubmitMsg into the section, mirroring the runtime submit path.
func submitOpenForm(t *testing.T, m *Model) {
	t.Helper()
	deliver(t, m, m.updateDialog(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}))
}

func TestSelectTogglesMarkAndRendersMarker(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.Init(ctx) // sizes the list so View renders rows
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "a"), mkIssue("JCT-2", "To Do", "b")}
	m.applyFilter()

	// Space on the first (selected) row marks it.
	m.Update(space())
	if !m.marks["JCT-1"] {
		t.Fatal("space did not mark the selected issue")
	}
	if got := m.markedKeys(); len(got) != 1 || got[0] != "JCT-1" {
		t.Errorf("markedKeys = %v, want [JCT-1]", got)
	}
	if out := m.View(); !strings.Contains(out, "✓") {
		t.Errorf("marked row not flagged with ✓ in view:\n%s", out)
	}

	// Space again clears the mark.
	m.Update(space())
	if m.marks["JCT-1"] {
		t.Error("second space did not unmark")
	}
}

func TestTransitionWithMarksOpensBulkPicker(t *testing.T) {
	svc := fakeIssueSvc{transitions: []*jira.Transition{mkTransition("11", "Done")}}
	ctx := newTestCtx(fakeServices{issue: svc})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "a")}
	m.applyFilter()
	m.marks = map[string]bool{"JCT-1": true}

	// 't' with a selection fetches the representative issue's transitions and
	// opens the same picker as a single transition — no free-text status entry.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 't'})
	if cmd == nil {
		t.Fatal("t with marks did not fetch transitions")
	}
	m.Update(cmd())
	if !m.pickOpen() {
		t.Fatalf("expected bulk-transition pick, got active=%v top=%T", m.dialogs.Active(), m.dialogs.Top())
	}
}

func TestRefetchPrunesStaleMarks(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "a"), mkIssue("JCT-2", "To Do", "b")}
	m.applyFilter()
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true}

	// A refetch that no longer contains JCT-1 must drop its stale mark while
	// keeping the mark for the surviving issue.
	m.handleTask(core.TaskFinishedMsg{
		Scope:  m.fetchScope(),
		Result: fetchResult{issues: []*jira.Issue{mkIssue("JCT-2", "To Do", "b")}},
	})
	if m.marks["JCT-1"] {
		t.Error("stale mark for removed issue not pruned")
	}
	if !m.marks["JCT-2"] {
		t.Error("mark for surviving issue wrongly dropped")
	}
}

func TestBulkTransitionAllSucceed(t *testing.T) {
	rec := &callRecorder{}
	svc := fakeIssueSvc{
		rec: rec,
		transitionsByKey: map[string][]*jira.Transition{
			"JCT-1": {mkTransition("11", "Done")},
			"JCT-2": {mkTransition("12", "Done")},
		},
	}
	ctx := newTestCtx(fakeServices{issue: svc})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{mkIssue("JCT-1", "To Do", "a"), mkIssue("JCT-2", "To Do", "b")}
	m.applyFilter()
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true}

	// Drive the real submit path: open the bulk transition pick, take the
	// selected transition, then approve the confirmation prompt.
	m.handleTask(core.TaskFinishedMsg{
		Scope:  m.transitionsScope(),
		Result: transitionsResult{bulk: true, transitions: []*jira.Transition{mkTransition("11", "Done")}},
	})
	m.passGrace()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick the only transition → parks the confirm
	p := m.confirmPrompt()
	if p == "" {
		t.Fatal("bulk pick must park behind the confirmation prompt")
	}
	if !strings.Contains(p, "2 issues") || !strings.Contains(p, `"Done"`) {
		t.Fatalf("confirm prompt = %q, want count and status", p)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !m.bulkPending {
		t.Fatal("confirming did not mark the bulk batch in flight")
	}
	// tea.Cmd is a plain function: each call re-runs the whole batch, so invoke it
	// exactly once and feed that message to Update.
	msg, ok := cmd().(core.TaskFinishedMsg)
	if !ok {
		t.Fatal("bulk transition did not produce a TaskFinishedMsg")
	}
	sec, _ := m.Update(msg)
	m = sec.(*Model)
	if m.flash.Msg != "2 → Done" {
		t.Errorf("flash = %q, want '2 → Done'", m.flash.Msg)
	}
	if len(m.marks) != 0 {
		t.Errorf("marks not cleared after bulk transition: %v", m.marks)
	}
	if m.bulkPending {
		t.Error("bulkPending not cleared after completion")
	}
	// Each issue must post its own resolved transition id, not a shared one.
	if got := rec.get("JCT-1"); got != "11" {
		t.Errorf("JCT-1 posted id = %q, want 11", got)
	}
	if got := rec.get("JCT-2"); got != "12" {
		t.Errorf("JCT-2 posted id = %q, want 12", got)
	}
}

// bulkModel builds a sized model with three issues loaded the way a fetch
// would land them.
func bulkModel(t *testing.T, svc fakeServices) *Model {
	t.Helper()
	ctx := newTestCtx(svc)
	m := New(ctx).(*Model)
	m.Init(ctx)
	land(
		m,
		mkIssue("JCT-1", "To Do", "a"),
		mkIssue("JCT-2", "To Do", "b"),
		mkIssue("JCT-3", "To Do", "c"),
	)
	return m
}

func TestSelectAllTogglesBetweenAllAndNone(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if got := len(m.markedKeys()); got != 3 {
		t.Fatalf("select-all marked %d, want 3", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if got := len(m.markedKeys()); got != 0 {
		t.Errorf("second select-all kept %d marks, want none", got)
	}
}

func TestSelectAllMarksOnlyTheFilteredView(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.filter = "JCT-2"
	m.applyFilter()
	m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if got := m.markedKeys(); len(got) != 1 || got[0] != "JCT-2" {
		t.Errorf("select-all under filter marked %v, want [JCT-2]", got)
	}
}

func TestSelectInvertFlipsEveryShownRow(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.marks = map[string]bool{"JCT-1": true}
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	got := m.markedKeys()
	if len(got) != 2 || got[0] != "JCT-2" || got[1] != "JCT-3" {
		t.Errorf("invert marked %v, want [JCT-2 JCT-3]", got)
	}
}

func TestSelectRangeMarksAnchorThroughCursor(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.Update(space()) // anchor on JCT-1
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor on JCT-3
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.markedKeys(); len(got) != 3 {
		t.Errorf("range marked %v, want all three", got)
	}
}

func TestBulkConfirmAnyOtherKeyCancelsAndKeepsMarks(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true}
	m.handleTask(core.TaskFinishedMsg{
		Scope:  m.transitionsScope(),
		Result: transitionsResult{bulk: true, transitions: []*jira.Transition{mkTransition("11", "Done")}},
	})
	m.passGrace()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick → parks the confirm
	if m.confirmPrompt() == "" || !m.CapturesInput() {
		t.Fatal("pick did not open the confirmation")
	}
	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.dialogs.Active() || m.bulkPending {
		t.Fatal("n did not cancel the bulk request")
	}
	if len(m.marks) != 2 {
		t.Errorf("cancel dropped the selection: %v", m.marks)
	}
}

func TestBulkAssignAppliesResolvedUserToEveryMark(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{writes: w}, user: fakeUserSvc{resolved: "acc-bob"}}
	m := bulkModel(t, svc)
	m.marks = map[string]bool{"JCT-1": true, "JCT-3": true}

	m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if f := m.activeForm(); f == nil || f.ctrl.Mode() != action.ModeBulkAssign {
		t.Fatal("a with marks did not open the bulk assign form")
	}
	m.dialogs.Update(tea.KeyPressMsg{Text: "bob"})
	submitOpenForm(t, m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	msg := cmd().(core.TaskFinishedMsg)
	res := msg.Result.(bulkResult)
	if len(res.succeeded) != 2 || res.summary() != "2 → @bob" {
		t.Errorf("bulk assign result = %+v, summary %q", res, res.summary())
	}
	for _, key := range []string{"JCT-1", "JCT-3"} {
		if got := w.get("assignee:" + key); got != "acc-bob" {
			t.Errorf("%s assignee = %q, want acc-bob", key, got)
		}
	}
}

func TestBulkCommentPostsToEveryMark(t *testing.T) {
	w := &callRecorder{}
	svc := fakeServices{issue: fakeIssueSvc{writes: w}}
	m := bulkModel(t, svc)
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true}

	m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if f := m.activeForm(); f == nil || f.ctrl.Mode() != action.ModeBulkComment {
		t.Fatal("c with marks did not open the bulk comment form")
	}
	m.dialogs.Update(tea.KeyPressMsg{Text: "ping"})
	submitOpenForm(t, m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	msg := cmd().(core.TaskFinishedMsg)
	res := msg.Result.(bulkResult)
	if len(res.succeeded) != 2 || res.summary() != "2 commented" {
		t.Errorf("bulk comment result = %+v, summary %q", res, res.summary())
	}
	for _, key := range []string{"JCT-1", "JCT-2"} {
		if got := w.get("comment:" + key); got != "1" {
			t.Errorf("%s comment not posted", key)
		}
	}
}

func TestBulkCommentTextIsNotTrimmed(t *testing.T) {
	w := &callRecorder{}
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.marks = map[string]bool{"JCT-1": true}
	m.openTextForm(action.ModeBulkComment, "", "")
	m.dialogs.Update(tea.KeyPressMsg{Text: "    indented code"})
	submitOpenForm(t, m)
	if m.confirmPrompt() == "" {
		t.Fatal("bulk comment did not park behind the confirmation")
	}
	// Approve and run the batch: the leading whitespace must reach the
	// Markdown conversion, so the posted body is a code block, not a paragraph
	// of trimmed prose.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m.Update(cmd())
	if got := w.get("comment-node:JCT-1"); got != "codeBlock" {
		t.Fatalf("posted body's first node = %q, want codeBlock (whitespace kept)", got)
	}
}

func TestEmptyBulkAssignReachesUnassignConfirm(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true}
	m.openTextForm(action.ModeBulkAssign, "", "")
	submitOpenForm(t, m) // empty query means unassign
	p := m.confirmPrompt()
	if p == "" {
		t.Fatal("empty bulk assign did not reach the confirmation")
	}
	if !strings.Contains(p, "Unassign 2 issues") {
		t.Errorf("prompt = %q, want unassign wording", p)
	}
}

func TestTransitionKeyBlockedWhileSingleWriteInFlight(t *testing.T) {
	m := bulkModel(t, fakeServices{issue: fakeIssueSvc{}})
	m.marks = map[string]bool{"JCT-1": true}
	m.writing = true // a comment/assign/edit is reconciling
	m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if m.dialogs.Active() {
		t.Error("t opened a bulk transition while a write was in flight")
	}
}

func TestBulkTransitionPartialFailureDoesNotAbortBatch(t *testing.T) {
	// JCT-1 can reach Done; JCT-2 has no matching transition; JCT-3's apply fails.
	svc := fakeIssueSvc{
		transitionsByKey: map[string][]*jira.Transition{
			"JCT-1": {mkTransition("11", "Done")},
			"JCT-2": {mkTransition("21", "In Review")},
			"JCT-3": {mkTransition("31", "Done")},
		},
		transErrByKey: map[string]error{"JCT-3": errBoom},
	}
	ctx := newTestCtx(fakeServices{issue: svc})
	m := New(ctx).(*Model)
	m.all = []*jira.Issue{
		mkIssue("JCT-1", "To Do", "a"),
		mkIssue("JCT-2", "To Do", "b"),
		mkIssue("JCT-3", "To Do", "c"),
	}
	m.applyFilter()
	m.marks = map[string]bool{"JCT-1": true, "JCT-2": true, "JCT-3": true}

	cmd := m.bulkTransition(m.markedKeys(), "Done")
	msg := cmd().(core.TaskFinishedMsg)
	if msg.Err != nil {
		t.Fatalf("a partial failure must not fail the whole task: %v", msg.Err)
	}
	res, ok := msg.Result.(bulkResult)
	if !ok {
		t.Fatalf("result is %T, want bulkResult", msg.Result)
	}
	if len(res.succeeded) != 1 || res.succeeded[0] != "JCT-1" {
		t.Errorf("succeeded = %v, want [JCT-1]", res.succeeded)
	}
	if len(res.failed) != 2 {
		t.Errorf("failed = %v, want JCT-2 and JCT-3", res.failed)
	}
	if got := res.summary(); got != "1 → Done · failed: JCT-2, JCT-3" {
		t.Errorf("summary = %q, want '1 → Done · failed: JCT-2, JCT-3'", got)
	}
}
