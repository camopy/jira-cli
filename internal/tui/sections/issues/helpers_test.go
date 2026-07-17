package issues

import "time"

// activeForm returns the open text/create form dialog, or nil when the top
// dialog is not one (or nothing is open) — the seam the tests use to reach the
// controller state that lives inside the dialog on the stack. Production code
// drives the form through the stack's messages instead.
func (r *results) activeForm() *formDialog {
	if d, ok := r.dialogs.Top().(*formDialog); ok {
		return d
	}
	return nil
}

// confirmPrompt returns the open confirmation dialog's prompt text, or ""
// when the top dialog is not a section confirm — how the tests observe that a
// bulk write parked behind its y/N guard now that the request rides the
// dialog's bound action.
func (r *results) confirmPrompt() string {
	if c, ok := r.dialogs.Top().(sectionConfirm); ok {
		return c.Content(0)
	}
	return ""
}

// pickOpen reports whether the top dialog is a section pick — the tests'
// stand-in for the old kind discriminator.
func (r *results) pickOpen() bool {
	_, ok := r.dialogs.Top().(sectionPick)
	return ok
}

// passGrace jumps the dialog stack's clock past the async-open input grace so
// the test's next key drives the dialog instead of being absorbed as
// in-flight. The offset must exceed the dialog package's grace ceiling
// (1.5s); the clock stays live rather than frozen so real elapsed time still
// moves it.
func (r *results) passGrace() {
	r.dialogs.SetClock(func() time.Time { return time.Now().Add(2 * time.Second) })
}
