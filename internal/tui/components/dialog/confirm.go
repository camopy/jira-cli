package dialog

import (
	tea "charm.land/bubbletea/v2"

	pkey "github.com/gechr/primer/key"
)

// Confirm is a generic y/N confirmation dialog. It renders a prompt as its
// body and resolves to ResultSubmit when accepted or ResultClose when
// declined, exposing the decision through Confirmed. It defaults to not
// confirmed, so a dialog abandoned any other way — escape, or a lost overlay
// the Stack drops — is read as a decline, the safe default for a guard in
// front of a destructive action.
//
// It carries pointer semantics: Update mutates and returns the same value, so
// the caller reads Confirmed off the very pointer the Stack pops.
type Confirm struct {
	prompt    string
	confirmed bool
}

// NewConfirm returns a Confirm that asks prompt, initially not confirmed.
func NewConfirm(prompt string) *Confirm {
	return &Confirm{prompt: prompt}
}

// Confirmed reports whether the dialog was accepted. It is meaningful once the
// Stack has popped the dialog: a Confirm resolved with ResultSubmit reports
// true, and one that was closed — or is still open — reports false.
func (c *Confirm) Confirmed() bool { return c.confirmed }

// Title omits a heading; the prompt carries the whole message.
func (c *Confirm) Title() string { return "" }

// Content renders the prompt as the body. The Shell frames, centers, and wraps
// it, so Confirm is box-less and draws no border of its own.
func (c *Confirm) Content(int) string { return c.prompt }

// Hints advertises the accept and decline keys for the Shell's foot row.
func (c *Confirm) Hints() []pkey.Hint {
	return []pkey.Hint{
		{Key: "y", Desc: "yes"},
		{Key: "n", Desc: "no"},
	}
}

// Update resolves the dialog on a decision key and ignores everything else.
// y/Y/enter accept and submit; n/N/esc decline and close. The outcome is
// recorded before returning, so the popped value already carries it.
func (c *Confirm) Update(msg tea.Msg) (Dialog, tea.Cmd, Result) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil, ResultNone
	}
	switch key.String() {
	case "y", "Y", "enter":
		c.confirmed = true
		return c, nil, ResultSubmit
	case "n", "N", "esc":
		c.confirmed = false
		return c, nil, ResultClose
	}
	return c, nil, ResultNone
}
