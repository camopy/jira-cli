package issues

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
)

func TestOpenCommentAlwaysOpensOverlay(t *testing.T) {
	// Even with an editor configured the overlay is the default; the editor
	// is reached explicitly via ctrl+e from inside it.
	t.Setenv("JIRA_EDITOR", "true")
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	if cmd := m.openComment(); cmd != nil {
		t.Fatal("openComment bypassed the overlay for the external editor")
	}
	if !m.ctrl.Active() || m.ctrl.Mode() != action.ModeComment {
		t.Fatal("modal comment not open")
	}
}

func TestCommentEditorHatchCarriesDraft(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "true")
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.openComment()
	m.ctrl.Update(tea.KeyPressMsg{Text: "started in the modal"})
	cmd := m.updateAction(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+e did not launch the editor")
	}
	// The overlay stays open across the handoff: it still holds the draft, so
	// a failed editor launch loses nothing. handleEditor closes it.
	if !m.ctrl.Active() {
		t.Error("overlay closed before the editor round-trip resolved")
	}
}

func TestEditorFailureKeepsOverlayAndDraft(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "true")
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.openComment()
	m.ctrl.Update(tea.KeyPressMsg{Text: "precious draft"})
	m.updateAction(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m.handleEditor(input.EditorFinishedMsg{ID: "comment:JCT-1", Err: errors.New("editor exploded")})
	if !m.ctrl.Active() || m.ctrl.Draft() != "precious draft" {
		t.Fatalf("editor failure lost the draft: active=%v draft=%q", m.ctrl.Active(), m.ctrl.Draft())
	}
}

func TestEditorSuccessClosesOverlay(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "true")
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.openComment()
	m.ctrl.Update(tea.KeyPressMsg{Text: "draft"})
	m.updateAction(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	cmd := m.handleEditor(input.EditorFinishedMsg{ID: "comment:JCT-1", Text: "final text"})
	if m.ctrl.Active() {
		t.Error("overlay still open after the editor delivered the comment")
	}
	if cmd == nil {
		t.Fatal("editor text did not produce a comment mutation")
	}
	m.Update(cmd())
	if w.get("comment:JCT-1") != "1" {
		t.Errorf("AddComment not called; recorder=%v", w.posted)
	}
}

func TestOpenCommentNoEditorStillOverlay(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "")
	t.Setenv("EDITOR", "")
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	if cmd := m.openComment(); cmd != nil {
		t.Fatal("no editor configured: comment must stay in the modal")
	}
	if !m.ctrl.Active() || m.ctrl.Mode() != action.ModeComment {
		t.Error("modal comment not open")
	}
}

func TestEditorFinishedCommentPosts(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	cmd := m.handleEditor(input.EditorFinishedMsg{ID: "comment:JCT-1", Text: "from the editor"})
	if cmd == nil {
		t.Fatal("editor text did not produce a comment mutation")
	}
	m.Update(cmd())
	if w.get("comment:JCT-1") != "1" {
		t.Errorf("AddComment not called; recorder=%v", w.posted)
	}
}

func TestEditorFinishedEmptyBufferDiscards(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.handleEditor(input.EditorFinishedMsg{ID: "comment:JCT-1", Text: "  \n"})
	if w.get("comment:JCT-1") != "" {
		t.Fatal("blank editor buffer must not post a comment")
	}
	if m.flash.Msg != "empty comment discarded" {
		t.Errorf("flash = %q, want discard notice", m.flash.Msg)
	}
}

func TestEditorFinishedErrorSurfaces(t *testing.T) {
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{}})
	wantErr := errors.New("editor exploded")
	if cmd := m.handleEditor(input.EditorFinishedMsg{ID: "comment:JCT-1", Err: wantErr}); cmd != nil {
		t.Fatal("a failed editor run must not post")
	}
	if !errors.Is(m.err, wantErr) {
		t.Errorf("err = %v, want the editor error surfaced", m.err)
	}
}

func TestEditorFinishedForeignKindIgnored(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	if cmd := m.handleEditor(input.EditorFinishedMsg{ID: "edit:JCT-1", Text: "x"}); cmd != nil {
		t.Error("non-comment editor result must be ignored for now")
	}
}

func TestPasteRoutesIntoOpenAction(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.ctrl.OpenText(action.ModeEdit, "JCT-1", "")
	m.Update(tea.PasteMsg{Content: "pasted summary"})
	if got := m.ctrl.Draft(); got != "pasted summary" {
		t.Errorf("action text = %q, want pasted content", got)
	}
}

func TestPasteRoutesIntoFilter(t *testing.T) {
	w := &callRecorder{}
	m := newVerbModel(t, fakeServices{issue: fakeIssueSvc{writes: w}})
	m.filtering = true
	m.filterInput = input.NewLine("/", "")
	m.Update(tea.PasteMsg{Content: "old"})
	if m.filter != "old" {
		t.Errorf("filter = %q, want %q (paste must re-apply the filter)", m.filter, "old")
	}
}
