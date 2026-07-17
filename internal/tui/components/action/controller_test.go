package action

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/components/form"
)

func TestCommentTextRoundTrips(t *testing.T) {
	var c Controller
	c.OpenText(ModeComment, "JCT-2", "")
	c.Update(tea.KeyPressMsg{Text: "looks goof"})
	c.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	c.Update(tea.KeyPressMsg{Text: "d"})
	req, ok := c.Request()
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
	if _, ok := c.Request(); ok {
		t.Error("empty comment was submitted")
	}
	if !c.Active() {
		t.Error("controller should stay open after a rejected submit")
	}
}

func TestCancelClears(t *testing.T) {
	var c Controller
	c.OpenText(ModeEdit, "JCT-3", "old summary")
	// A pristine draft needs no discard confirmation: esc cancels outright.
	if _, outcome := c.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); outcome != OutcomeCancel {
		t.Fatal("esc on a clean draft did not cancel")
	}
	if c.Active() || c.Mode() != ModeNone {
		t.Error("cancel did not close the controller")
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

// Stepping the project pill reports OutcomeChanged so the owner can refetch the
// new project's types; SetTypeOptions swaps them in, and the Request carries the
// chosen project and type.
func TestCreateProjectPillDrivesTypeRefetch(t *testing.T) {
	var c Controller
	c.OpenCreate(CreateConfig{
		Project:     "JCT",
		Projects:    []string{"JCT", "PROJ"},
		IssueTypes:  []string{"Task", "Bug"},
		DefaultType: "Task",
	})

	// Stepping the project pill reports OutcomeChanged and the new target.
	_, outcome := c.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if outcome != OutcomeChanged {
		t.Fatalf("stepping the project pill = %v, want OutcomeChanged", outcome)
	}
	if got := c.Project(); got != "PROJ" {
		t.Fatalf("Project() = %q, want PROJ", got)
	}

	// The owner refetches and pushes the new project's types back; the type field
	// adopts them, snapping to the first when the old value is gone.
	c.SetTypeOptions([]string{"Story", "Epic"})
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // project → type
	c.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // Story → Epic
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // → summary
	c.Update(tea.KeyPressMsg{Text: "New issue"})
	if _, outcome := c.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}); outcome != OutcomeSubmit {
		t.Fatalf("ctrl+s outcome = %v, want OutcomeSubmit", outcome)
	}
	req, ok := c.Request()
	if !ok {
		t.Fatal("create request reported incomplete")
	}
	if req.IssueKey != "PROJ" || req.IssueType != "Epic" {
		t.Errorf("req project/type = %q/%q, want PROJ/Epic", req.IssueKey, req.IssueType)
	}
}

// The create form collects the picked type, an accepted assignee's accountId,
// and parsed labels into the Request — the payload the section turns into a
// create call without any re-resolve.
func TestCreateFormBuildsRequest(t *testing.T) {
	var c Controller
	c.OpenCreate(CreateConfig{
		Project:     "JCT",
		IssueTypes:  []string{"Task", "Bug", "Story"},
		DefaultType: "Task",
		AssigneeFetch: func(string) []form.Suggestion {
			return []form.Suggestion{{Value: "Alice Smith", Label: "Alice Smith", Detail: "acc-99"}}
		},
	})
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // project → type
	c.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // type: Task → Bug
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // → summary
	c.Update(tea.KeyPressMsg{Text: "Ship it"})
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // → assignee
	c.Update(tea.KeyPressMsg{Text: "ali"})
	// The assignee fetch is async; deliver the suggestion the live query awaits.
	c.Update(form.SuggestionsMsg{Field: createFieldAssignee, Query: "ali", Items: []form.Suggestion{
		{Value: "Alice Smith", Label: "Alice Smith", Detail: "acc-99"},
	}})
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // accept the assignee
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // → labels
	c.Update(tea.KeyPressMsg{Text: "ux, backend"})

	_, outcome := c.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if outcome != OutcomeSubmit {
		t.Fatalf("ctrl+s outcome = %v, want OutcomeSubmit", outcome)
	}
	req, ok := c.Request()
	if !ok {
		t.Fatal("create request reported incomplete")
	}
	if req.IssueKey != "JCT" {
		t.Errorf("req project = %q, want the pill's JCT", req.IssueKey)
	}
	if req.Summary != "Ship it" || req.IssueType != "Bug" {
		t.Errorf("req summary/type = %q/%q, want 'Ship it'/'Bug'", req.Summary, req.IssueType)
	}
	if req.Assignee != "acc-99" {
		t.Errorf("req assignee = %q, want the accepted accountId acc-99", req.Assignee)
	}
	if len(req.Labels) != 2 || req.Labels[0] != "ux" || req.Labels[1] != "backend" {
		t.Errorf("req labels = %v, want [ux backend]", req.Labels)
	}
}
