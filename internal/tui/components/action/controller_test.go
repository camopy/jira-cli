package action

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/jira"
)

func transition(id, name string) *jira.Transition {
	return &jira.Transition{ID: &id, Name: &name}
}

func TestTransitionSubmitReturnsIDAndName(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{
		transition("11", "To Do"),
		transition("21", "In Progress"),
		transition("41", "Done"),
	})
	if !c.Active() || c.Mode() != ModeTransition {
		t.Fatal("controller not in transition mode after open")
	}
	c.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // → In Progress

	req, ok := c.Submit()
	if !ok {
		t.Fatal("submit reported incomplete")
	}
	if req.TransitionID != "21" || req.TransitionName != "In Progress" {
		t.Errorf("submitted %q/%q, want 21/In Progress", req.TransitionID, req.TransitionName)
	}
	if c.Active() {
		t.Error("controller still active after submit")
	}
}

func TestTransitionNavigationClampsAtBounds(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do"), transition("41", "Done")})
	for i := 0; i < 5; i++ {
		c.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if req, ok := c.Submit(); !ok || req.TransitionID != "11" {
		t.Errorf("choice underflow submitted %q, want 11", req.TransitionID)
	}
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do"), transition("41", "Done")})
	for i := 0; i < 5; i++ {
		c.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if req, ok := c.Submit(); !ok || req.TransitionID != "41" {
		t.Errorf("choice overflow submitted %q, want 41", req.TransitionID)
	}
}

func TestTransitionPickerFiltersOnTyping(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{
		transition("11", "To Do"),
		transition("21", "In Progress"),
		transition("41", "Done"),
	})
	for _, r := range "done" {
		c.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	req, ok := c.Submit()
	if !ok || req.TransitionID != "41" || req.TransitionName != "Done" {
		t.Errorf("filtered submit = %+v, %v; want Done/41", req, ok)
	}
}

func TestTransitionPickerNoMatchCannotSubmit(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do")})
	c.Update(tea.KeyPressMsg{Text: "zzz"})
	if _, ok := c.Submit(); ok {
		t.Error("submit succeeded with no transition matching the filter")
	}
	if !c.Active() {
		t.Error("controller should stay open after a rejected submit")
	}
}

func TestEmptyTransitionListCannotSubmit(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", nil)
	if _, ok := c.Submit(); ok {
		t.Error("submit succeeded with no valid transitions")
	}
}

func TestCommentTextRoundTrips(t *testing.T) {
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "")
	c.Update(tea.KeyPressMsg{Text: "looks goof"})
	c.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	c.Update(tea.KeyPressMsg{Text: "d"})
	req, ok := c.Submit()
	if !ok {
		t.Fatal("comment submit reported incomplete")
	}
	if req.Mode != ModeComment || req.Text != "looks good" {
		t.Errorf("comment req = %+v, want text 'looks good'", req)
	}
}

func TestEmptyCommentCannotSubmit(t *testing.T) {
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "   ")
	if _, ok := c.Submit(); ok {
		t.Error("empty comment was submitted")
	}
	if !c.Active() {
		t.Error("controller should stay open after a rejected submit")
	}
}

func TestCancelClears(t *testing.T) {
	var c Controller
	c.OpenText(ModeEdit, "JCT-3", "old summary")
	c.Cancel()
	if c.Active() || c.Mode() != ModeNone {
		t.Error("cancel did not close the controller")
	}
}

func TestTypeIgnoredOutsideTextMode(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do")})
	c.Update(tea.KeyPressMsg{Text: "xyz"}) // must be ignored in a choice mode
	if got := c.Draft(); got != "" {
		t.Errorf("text captured in transition mode: %q", got)
	}
}

func TestPickerEscCancels(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do")})
	_, outcome := c.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if outcome != OutcomeCancel || c.Active() {
		t.Errorf("esc on picker: outcome=%v active=%v, want cancel+closed", outcome, c.Active())
	}
}

func TestPickerEnterReportsSubmit(t *testing.T) {
	var c Controller
	c.OpenTransition("JCT-1", []*jira.Transition{transition("11", "To Do")})
	_, outcome := c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if outcome != OutcomeSubmit {
		t.Errorf("enter on picker: outcome=%v, want submit", outcome)
	}
	if req, ok := c.Submit(); !ok || req.TransitionID != "11" {
		t.Errorf("submit after enter = %+v, %v", req, ok)
	}
}

func TestDirtyCommentEscGuardsBeforeCancel(t *testing.T) {
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "")
	c.Update(tea.KeyPressMsg{Text: "half a thought"})
	if _, outcome := c.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); outcome != OutcomeNone || !c.Active() {
		t.Fatal("dirty esc dropped the draft without asking")
	}
	// y confirms the discard and closes the action.
	if _, outcome := c.Update(tea.KeyPressMsg{Text: "y", Code: 'y'}); outcome != OutcomeCancel || c.Active() {
		t.Fatal("confirmed discard did not cancel")
	}
}

func TestCommentEditorHatchReportsOutcome(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "true")
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "")
	c.Update(tea.KeyPressMsg{Text: "draft so far"})
	_, outcome := c.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if outcome != OutcomeEditor {
		t.Fatalf("ctrl+e outcome = %v, want editor", outcome)
	}
	if got := c.Draft(); got != "draft so far" {
		t.Errorf("draft = %q; the editor handoff would lose text", got)
	}
	if c.IssueKey() != "JCT-2" {
		t.Errorf("issue key = %q", c.IssueKey())
	}
}

func TestCommentNoEditorNoHatch(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "")
	t.Setenv("EDITOR", "")
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "")
	if _, outcome := c.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}); outcome != OutcomeNone {
		t.Errorf("ctrl+e with no editor configured: outcome=%v, want none", outcome)
	}
}

func TestBulkCommentHasNoEditorHatch(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "true")
	var c Controller
	c.OpenText(ModeBulkComment, "", "")
	if _, outcome := c.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}); outcome != OutcomeNone {
		t.Errorf("bulk comment offered the editor hatch: outcome=%v", outcome)
	}
}
