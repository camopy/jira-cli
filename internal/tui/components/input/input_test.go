package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeText(t *testing.T, update func(tea.Msg) tea.Cmd, text string) {
	t.Helper()
	update(tea.KeyPressMsg{Text: text})
}

func TestLineTypesAndBackspaces(t *testing.T) {
	l := NewLine("> ", "hint")
	typeText(t, l.Update, "abcd")
	l.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := l.Value(); got != "abc" {
		t.Errorf("Value() = %q, want %q", got, "abc")
	}
}

func TestLineSetValueAppendsAtEnd(t *testing.T) {
	l := NewLine("", "")
	l.SetValue("project = JCT")
	typeText(t, l.Update, "!")
	if got := l.Value(); got != "project = JCT!" {
		t.Errorf("typing after SetValue: Value() = %q, cursor not at end", got)
	}
}

func TestAreaCollectsMultilineText(t *testing.T) {
	a := NewArea("hint", 40, 4)
	typeText(t, a.Update, "first")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeText(t, a.Update, "second")
	if got := a.Value(); got != "first\nsecond" {
		t.Errorf("Value() = %q, want two lines", got)
	}
}

func TestEditorCommandPrecedence(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "nvim -f")
	t.Setenv("EDITOR", "vi")
	if got := EditorCommand(); got != "nvim -f" {
		t.Errorf("JIRA_EDITOR should win, got %q", got)
	}
	t.Setenv("JIRA_EDITOR", "")
	if got := EditorCommand(); got != "vi" {
		t.Errorf("EDITOR fallback, got %q", got)
	}
	t.Setenv("EDITOR", "")
	if got := EditorCommand(); got != "" {
		t.Errorf("no editor configured should be empty, got %q", got)
	}
}

func TestEditWithoutEditorReturnsError(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "")
	t.Setenv("EDITOR", "")
	cmd := Edit("comment:JCT-1", "")
	if cmd == nil {
		t.Fatal("Edit returned nil cmd")
	}
	msg, ok := cmd().(EditorFinishedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want EditorFinishedMsg", cmd())
	}
	if msg.ID != "comment:JCT-1" || msg.Err == nil {
		t.Errorf("msg = %+v, want routed ID and a configuration error", msg)
	}
}

func TestLineSetWidthClampsNonPositive(t *testing.T) {
	l := NewLine("/", "")
	l.SetWidth(-5) // a too-narrow pane must not panic the textinput
	typeText(t, l.Update, "ok")
	_ = l.View()
	if l.Value() != "ok" {
		t.Errorf("Value() = %q after clamped width", l.Value())
	}
}

func TestLinePasteInserts(t *testing.T) {
	l := NewLine("", "")
	l.Update(tea.PasteMsg{Content: "pasted text"})
	if l.Value() != "pasted text" {
		t.Errorf("paste not inserted: %q", l.Value())
	}
}
