package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirm(t *testing.T) {
	cases := []struct {
		name          string
		key           tea.KeyPressMsg
		wantResult    Result
		wantConfirmed bool
	}{
		{name: "lowercase y submits confirmed", key: tea.KeyPressMsg{Text: "y", Code: 'y'}, wantResult: ResultSubmit, wantConfirmed: true},
		{name: "uppercase y submits confirmed", key: tea.KeyPressMsg{Text: "Y", Code: 'y', Mod: tea.ModShift}, wantResult: ResultSubmit, wantConfirmed: true},
		{name: "enter submits confirmed", key: tea.KeyPressMsg{Code: tea.KeyEnter}, wantResult: ResultSubmit, wantConfirmed: true},
		{name: "lowercase n closes not confirmed", key: tea.KeyPressMsg{Text: "n", Code: 'n'}, wantResult: ResultClose, wantConfirmed: false},
		{name: "uppercase n closes not confirmed", key: tea.KeyPressMsg{Text: "N", Code: 'n', Mod: tea.ModShift}, wantResult: ResultClose, wantConfirmed: false},
		{name: "esc closes not confirmed", key: tea.KeyPressMsg{Code: tea.KeyEscape}, wantResult: ResultClose, wantConfirmed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewConfirm("delete everything?")
			next, cmd, res := c.Update(tc.key)
			if res != tc.wantResult {
				t.Fatalf("result = %v, want %v", res, tc.wantResult)
			}
			if cmd != nil {
				t.Fatalf("Update returned a command, want nil")
			}
			// Update returns the same pointer so the caller reads the outcome off
			// the value the Stack pops.
			if next.(*Confirm) != c {
				t.Fatal("Update returned a different value than the receiver")
			}
			if c.Confirmed() != tc.wantConfirmed {
				t.Fatalf("Confirmed() = %v, want %v", c.Confirmed(), tc.wantConfirmed)
			}
		})
	}
}

func TestConfirmDefaults(t *testing.T) {
	t.Run("defaults to not confirmed", func(t *testing.T) {
		if NewConfirm("proceed?").Confirmed() {
			t.Fatal("a fresh Confirm reports confirmed")
		}
	})

	t.Run("an unrelated key keeps it open and undecided", func(t *testing.T) {
		c := NewConfirm("proceed?")
		_, _, res := c.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
		if res != ResultNone {
			t.Fatalf("result = %v, want ResultNone", res)
		}
		if c.Confirmed() {
			t.Fatal("an unrelated key confirmed the dialog")
		}
	})

	t.Run("a non-key message is ignored", func(t *testing.T) {
		c := NewConfirm("proceed?")
		_, _, res := c.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		if res != ResultNone {
			t.Fatalf("result = %v, want ResultNone", res)
		}
	})

	t.Run("the prompt is rendered as the body", func(t *testing.T) {
		if got := NewConfirm("proceed?").Content(40); got != "proceed?" {
			t.Fatalf("Content = %q, want the prompt", got)
		}
	})
}
