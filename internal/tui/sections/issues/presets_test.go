package issues

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func presetModel(t *testing.T) *SearchModel {
	t.Helper()
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	return s
}

func ctrlP() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl} }

func TestCtrlPOpensPresetDropdownAndCapturesInput(t *testing.T) {
	s := presetModel(t)
	s.Update(ctrlP())
	if !s.pickOpen() {
		t.Fatalf("ctrl+p opened active=%v top=%T, want preset dropdown", s.dialogs.Active(), s.dialogs.Top())
	}
	if !s.CapturesInput() {
		t.Error("preset dropdown must capture input")
	}
}

func TestPresetEnterCommitsPickedJQLAndRuns(t *testing.T) {
	s := presetModel(t)
	s.Update(ctrlP())
	// Type to narrow to the Reported preset, then run it.
	for _, r := range "reported" {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.dialogs.Active() {
		t.Fatal("enter did not close the dropdown")
	}
	if !strings.Contains(s.jql, "reporter = currentUser()") {
		t.Errorf("committed jql = %q, want the Reported preset", s.jql)
	}
	if cmd == nil {
		t.Error("picking a preset did not run the query")
	}
}

func TestPresetEscClosesWithoutRunning(t *testing.T) {
	s := presetModel(t)
	s.Update(ctrlP())
	s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if s.dialogs.Active() || s.jql != "" {
		t.Errorf("esc left dropdown active=%v jql=%q", s.dialogs.Active(), s.jql)
	}
}

func TestCtrlPWorksWhileEditingJQL(t *testing.T) {
	s := presetModel(t)
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // start editing
	s.Update(ctrlP())
	if !s.pickOpen() {
		t.Fatal("ctrl+p during JQL edit did not open the dropdown")
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick first preset
	if s.jql == "" || s.editing {
		t.Errorf("preset from edit mode: jql=%q editing=%v", s.jql, s.editing)
	}
}

func TestJQLEditorOffersSuggestions(t *testing.T) {
	s := presetModel(t)
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // start editing
	// Type the start of a saved query; the ghost suggestion should appear in
	// the rendered input (the suggestion view shows the completion).
	for _, r := range "assignee" {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if v := s.jqlInput.View(); !strings.Contains(v, "currentUser()") {
		t.Errorf("editor view missing ghost suggestion:\n%q", v)
	}
	// Tab accepts the suggestion into the value.
	s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if v := s.jqlInput.Value(); !strings.Contains(v, "currentUser()") {
		t.Errorf("tab did not accept the suggestion: %q", v)
	}
}
