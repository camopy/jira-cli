package form

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
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

var (
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
)

func TestFocusRegionTracksFocus(t *testing.T) {
	// A titled three-field form: title (1 line) then one titled box per text
	// field (3 lines: top border, body, bottom border). The region top must land
	// on each box's top border as focus moves, matching View's own layout.
	m := New(Config{
		Title: "new issue",
		Fields: []FieldSpec{
			{Label: "summary"},
			{Label: "assignee"},
			{Label: "labels"},
		},
		Width: 40,
	})

	top, height, ok := m.FocusRegion()
	if !ok || top != 1 || height != 3 {
		t.Fatalf("first field region = (%d,%d,%v), want (1,3,true)", top, height, ok)
	}

	press(t, &m, tab) // focus the second field
	if top, _, _ := m.FocusRegion(); top != 4 {
		t.Fatalf("second field top = %d, want 4", top)
	}

	press(t, &m, tab) // focus the third field
	top, _, _ = m.FocusRegion()
	if top != 7 {
		t.Fatalf("third field top = %d, want 7", top)
	}
	// The reported top must be a real line in View, and everything above it must
	// be the two earlier blocks plus the title.
	if lines := strings.Split(m.View(), "\n"); !strings.Contains(lines[top], "labels") {
		t.Fatalf("line %d = %q, want the labels field", top, lines[top])
	}
}

func TestFocusRegionInertForm(t *testing.T) {
	var m Model
	if _, _, ok := m.FocusRegion(); ok {
		t.Fatal("an inert form must report no focus region")
	}
}

func TestCycleFieldStepsAndWraps(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Label: "type", Options: []string{"Task", "Bug", "Story"}, Initial: "Bug"}},
		Width:  40,
	})
	if got := m.Value(0); got != "Bug" {
		t.Fatalf("initial: got %q, want Bug", got)
	}
	press(t, &m, keyRight)
	if got := m.Value(0); got != "Story" {
		t.Fatalf("after right: got %q, want Story", got)
	}
	press(t, &m, keyRight) // wraps past the end
	if got := m.Value(0); got != "Task" {
		t.Fatalf("after wrap: got %q, want Task", got)
	}
	press(t, &m, keyLeft) // wraps past the start
	if got := m.Value(0); got != "Story" {
		t.Fatalf("after left wrap: got %q, want Story", got)
	}
}

func TestCycleFieldInitialUnknownStartsFirst(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Options: []string{"Task", "Bug"}, Initial: "Epic"}},
		Width:  40,
	})
	if got := m.Value(0); got != "Task" {
		t.Fatalf("unknown initial: got %q, want the first option Task", got)
	}
}

// A required cycle field is never blank, so a submit is never blocked on it.
func TestCycleFieldSubmitsWithoutBlocking(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Options: []string{"Task", "Bug"}}},
		Width:  40,
	})
	if ev := press(t, &m, enter); ev != EventSubmit {
		t.Fatalf("cycle-only form: enter emitted %v, want EventSubmit", ev)
	}
}

// A cycle field whose Initial matches no option (an unset profile default)
// still opens pristine: esc must cancel outright, not prompt to discard.
func TestCycleFieldUnsetInitialIsNotDirty(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Options: []string{"Task", "Epic"}, Initial: ""}},
		Width:  40,
	})
	if ev := press(t, &m, esc); ev != EventCancel {
		t.Fatalf("esc on a pristine unset-default form emitted %v, want EventCancel", ev)
	}
}

func TestCycleFieldViewShowsSelection(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Options: []string{"Task", "Bug"}, Initial: "Bug"}},
		Width:  40,
	})
	if v := m.View(); !strings.Contains(v, "‹ Bug ›") {
		t.Fatalf("view missing cycle marker for Bug:\n%s", v)
	}
}

// A field with an accepted suggestion exposes that suggestion's Detail (an
// accountId behind a display name), and a later edit invalidates it.
func TestAcceptedDetailTracksAcceptanceAndClearsOnEdit(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Autocomplete: &Autocomplete{
				MinQuery:   3,
				IsBoundary: func(rune) bool { return false }, // whole field is the query
				Fetch:      func(string) []Suggestion { return nil },
			},
		}},
		Width: 40,
	})
	typeText(t, &m, "ali")
	// The fetch runs async; feed the result the query is waiting on directly.
	m.Update(SuggestionsMsg{Field: 0, Query: "ali", Items: []Suggestion{
		{Value: "Alice Smith", Label: "Alice Smith", Detail: "acc-99"},
	}})
	press(t, &m, enter) // accept the highlighted suggestion
	if got := m.Value(0); got != "Alice Smith" {
		t.Fatalf("accepted value = %q, want the display name inserted", got)
	}
	if got := m.AcceptedDetail(0); got != "acc-99" {
		t.Fatalf("AcceptedDetail = %q, want the accountId acc-99", got)
	}
	typeText(t, &m, "x") // any edit invalidates the recorded detail
	if got := m.AcceptedDetail(0); got != "" {
		t.Fatalf("AcceptedDetail after edit = %q, want cleared", got)
	}
}

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

func TestValidateBlocksSubmitAndShowsInline(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{
			Placeholder: "duration",
			Validate: func(s string) error {
				if s != "2h" {
					return errors.New("not a duration")
				}
				return nil
			},
		}},
		Styles: Styles{Error: lipgloss.NewStyle()},
		Width:  40,
	})
	typeText(t, &m, "banana")
	if ev := press(t, &m, enter); ev != EventNone {
		t.Fatalf("invalid field submitted: %v", ev)
	}
	if !strings.Contains(m.View(), "not a duration") {
		t.Fatalf("validation message not rendered inline: %q", m.View())
	}
	// Editing clears the inline error; a valid value then submits.
	for range len("banana") {
		press(t, &m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if strings.Contains(m.View(), "not a duration") {
		t.Fatal("editing did not clear the inline error")
	}
	typeText(t, &m, "2h")
	if ev := press(t, &m, enter); ev != EventSubmit {
		t.Fatalf("valid value still blocked: %v", ev)
	}
}

func TestOptionalMarkerRendersOnLabel(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{
			{Label: "summary"},
			{Label: "description", Optional: true},
		},
		Width: 40,
	})
	v := m.View()
	if !strings.Contains(v, "description (optional)") {
		t.Fatalf("optional marker missing: %q", v)
	}
	if strings.Contains(v, "summary (optional)") {
		t.Fatalf("required field wrongly marked optional: %q", v)
	}
}

func TestSubmittingAndErrorFootRow(t *testing.T) {
	m := New(Config{
		Fields: []FieldSpec{{Placeholder: "text"}},
		Styles: Styles{Error: lipgloss.NewStyle()},
		Width:  40,
	})
	typeText(t, &m, "hi")
	m.SetSubmitting("*")
	v := m.View()
	if !strings.Contains(v, "submitting…") || !strings.Contains(v, "*") {
		t.Fatalf("submitting foot row not shown: %q", v)
	}
	// A frozen form swallows keys so a second submit can't fire mid-write.
	if ev := press(t, &m, ctrlS); ev != EventNone {
		t.Fatalf("frozen form emitted %v", ev)
	}
	if m.Value(0) != "hi" {
		t.Fatalf("frozen form was edited: %q", m.Value(0))
	}
	m.SetError("write failed")
	v = m.View()
	if !strings.Contains(v, "write failed") || strings.Contains(v, "submitting…") {
		t.Fatalf("error did not replace submitting state: %q", v)
	}
	if m.Value(0) != "hi" {
		t.Fatalf("draft lost on error: %q", m.Value(0))
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
