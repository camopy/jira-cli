package form

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(t *testing.T, m *Model, msg tea.KeyPressMsg) EventKind {
	t.Helper()
	_, ev, _ := m.Update(msg)
	return ev
}

func typeText(t *testing.T, m *Model, s string) {
	t.Helper()
	if ev := press(t, m, tea.KeyPressMsg{Text: s}); ev != EventNone {
		t.Fatalf("typing %q emitted event %v", s, ev)
	}
}

var (
	esc     = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter   = tea.KeyPressMsg{Code: tea.KeyEnter}
	tab     = tea.KeyPressMsg{Code: tea.KeyTab}
	ctrlS   = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	ctrlE   = tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	keyDn   = tea.KeyPressMsg{Code: tea.KeyDown}
	shTab   = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	letterY = tea.KeyPressMsg{Text: "y", Code: 'y'}
	letterN = tea.KeyPressMsg{Text: "n", Code: 'n'}
)

func twoField() Model {
	return New(Config{
		Title: "create in PROJ",
		Fields: []FieldSpec{
			{Label: "summary", Placeholder: "one line"},
			{Label: "description", Multiline: true, Optional: true},
		},
		Width: 40,
	})
}

func TestEnterOnLineAdvancesThenCtrlSSubmits(t *testing.T) {
	m := twoField()
	typeText(t, &m, "fix the flux")
	if ev := press(t, &m, enter); ev != EventNone {
		t.Fatalf("enter on first field submitted early: %v", ev)
	}
	typeText(t, &m, "sparks when engaged")
	if ev := press(t, &m, ctrlS); ev != EventSubmit {
		t.Fatalf("ctrl+s did not submit: %v", ev)
	}
	got := m.Values()
	if got[0] != "fix the flux" || got[1] != "sparks when engaged" {
		t.Fatalf("values landed in wrong fields: %q", got)
	}
}

func TestTabCyclesFocusBothWays(t *testing.T) {
	m := twoField()
	press(t, &m, tab)
	typeText(t, &m, "body")
	press(t, &m, shTab)
	typeText(t, &m, "title")
	if got := m.Values(); got[0] != "title" || got[1] != "body" {
		t.Fatalf("focus ring misrouted input: %q", got)
	}
}

func TestSubmitBlockedOnBlankRequiredField(t *testing.T) {
	m := twoField()
	press(t, &m, tab) // land on the optional description
	typeText(t, &m, "only a description")
	if ev := press(t, &m, ctrlS); ev != EventNone {
		t.Fatalf("blank required summary submitted: %v", ev)
	}
	// The failed submit re-focused the blank summary.
	typeText(t, &m, "now titled")
	if ev := press(t, &m, ctrlS); ev != EventSubmit {
		t.Fatal("submit still blocked after filling the required field")
	}
	if got := m.Values(); got[0] != "now titled" {
		t.Fatalf("refocus did not land on the required field: %q", got)
	}
}

func TestEnterSubmitsSingleLineForm(t *testing.T) {
	m := New(Config{Fields: []FieldSpec{{Placeholder: "who"}}, Width: 40})
	typeText(t, &m, "alice")
	if ev := press(t, &m, enter); ev != EventSubmit {
		t.Fatalf("enter on a single-line form did not submit: %v", ev)
	}
}

func TestEscOnPristineFormCancels(t *testing.T) {
	m := twoField()
	if ev := press(t, &m, esc); ev != EventCancel {
		t.Fatalf("esc on pristine form: %v", ev)
	}
}

func TestEscOnPrefilledUntouchedFormCancels(t *testing.T) {
	m := New(Config{Fields: []FieldSpec{{Initial: "bug, ux"}}, Width: 40})
	if ev := press(t, &m, esc); ev != EventCancel {
		t.Fatalf("untouched prefill should cancel clean: %v", ev)
	}
}

func TestDirtyEscAsksBeforeDiscarding(t *testing.T) {
	m := twoField()
	typeText(t, &m, "half-written thought")
	if ev := press(t, &m, esc); ev != EventNone {
		t.Fatalf("dirty esc dropped the draft outright: %v", ev)
	}
	if !strings.Contains(m.View(), "discard input?") {
		t.Fatal("confirmation question not rendered")
	}
	// n resumes editing with the draft intact.
	if ev := press(t, &m, letterN); ev != EventNone {
		t.Fatalf("n during confirm: %v", ev)
	}
	if m.Value(0) != "half-written thought" {
		t.Fatalf("draft lost on resume: %q", m.Value(0))
	}
	// A second esc re-asks; y abandons.
	press(t, &m, esc)
	if ev := press(t, &m, letterY); ev != EventCancel {
		t.Fatalf("y during confirm did not cancel: %v", ev)
	}
}

func TestConfirmSwallowsStrayKeys(t *testing.T) {
	m := twoField()
	typeText(t, &m, "draft")
	press(t, &m, esc)
	if ev := press(t, &m, tea.KeyPressMsg{Text: "q", Code: 'q'}); ev != EventNone {
		t.Fatalf("stray key during confirm emitted %v", ev)
	}
	if m.Value(0) != "draft" {
		t.Fatalf("stray key mutated the draft: %q", m.Value(0))
	}
}

func TestEditorHatchOnlyOnMultiline(t *testing.T) {
	m := New(Config{
		Fields:      []FieldSpec{{Placeholder: "title"}, {Multiline: true, Optional: true}},
		EditorHatch: true,
		Width:       40,
	})
	if ev := press(t, &m, ctrlE); ev != EventNone {
		t.Fatalf("ctrl+e on a one-line field requested the editor: %v", ev)
	}
	press(t, &m, tab)
	if ev := press(t, &m, ctrlE); ev != EventEditor {
		t.Fatalf("ctrl+e on the multiline field: %v", ev)
	}
}

func TestHintsFollowShape(t *testing.T) {
	single := New(Config{Fields: []FieldSpec{{}}, Width: 40})
	if v := single.View(); !strings.Contains(v, "submit") || strings.Contains(v, "next field") {
		t.Fatalf("single-field hints wrong: %q", v)
	}
	multi := New(Config{
		Fields:      []FieldSpec{{}, {Multiline: true}},
		EditorHatch: true,
		Width:       40,
	})
	v := multi.View()
	for _, want := range []string{"submit", "next field", "editor", "cancel"} {
		if !strings.Contains(v, want) {
			t.Fatalf("multi-field hints missing %q: %q", want, v)
		}
	}
}

func TestZeroValueIsInert(t *testing.T) {
	var m Model
	if m.Active() {
		t.Fatal("zero form reports active")
	}
	if _, ev, consumed := m.Update(enter); ev != EventNone || consumed {
		t.Fatal("zero form consumed a key")
	}
	if m.View() != "" {
		t.Fatal("zero form rendered content")
	}
}

func TestUpdateReportsConsumed(t *testing.T) {
	m := twoField()
	_, _, consumed := m.Update(tea.KeyPressMsg{Text: "x"})
	if !consumed {
		t.Fatal("open form did not consume a key")
	}
}
