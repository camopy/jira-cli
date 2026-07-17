// The text/create action overlay as a dialog-stack citizen: formDialog adapts
// an action.Controller to the dialog.Dialog contract, and the two messages that
// drive its async submit lifecycle. A single-issue write keeps the dialog open
// in a submitting state until handleTask reports the outcome; a bulk collect
// closes it and lets the section park its confirmation.

package issues

import (
	tea "charm.land/bubbletea/v2"

	pkey "github.com/gechr/primer/key"

	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
)

// formSubmitMsg carries a completed action out of the form dialog to the
// section: the dialog cannot reach the section directly, so it emits this on a
// command and the section's Update starts the write (single-issue text modes)
// or parks a bulk confirmation.
type formSubmitMsg struct{ req action.Request }

// formFinishMsg resolves an in-flight form submit. The section feeds it into
// the dialog stack from handleTask's mutate arm: a nil error closes the form
// (the write landed), a non-nil error clears the submitting state and shows the
// message inline with the draft intact. The form dialog owns it; any other
// dialog on the stack ignores a message it doesn't recognize.
type formFinishMsg struct{ err error }

// formProjectChangedMsg tells the section the create form's project pill stepped
// to a new value, so it can refetch that project's issue types. The dialog emits
// it on a command; the section fires the fetch and feeds the result back through
// formSetTypesMsg.
type formProjectChangedMsg struct{ project string }

// formSetTypesMsg carries a refetched project's issue types back into the open
// create form. The section feeds it into the dialog stack once the fetch lands;
// the form dialog swaps the type field's options in place. Any other dialog
// ignores it.
type formSetTypesMsg struct{ types []string }

// formDialog adapts an action.Controller onto the dialog stack so the text and
// create overlays live beside the pick and confirm dialogs. The controller
// renders its own box-less form (title, fields, inline errors, hints); the
// Shell frames it. spinFrame is the static spinner glyph the submitting foot
// row shows.
type formDialog struct {
	ctrl      action.Controller
	spinFrame string
}

// newFormDialog wraps an opened controller for the stack. spinFrame is the
// section spinner's current glyph, shown beside "submitting…" while a write is
// in flight.
func newFormDialog(ctrl action.Controller, spinFrame string) *formDialog {
	return &formDialog{ctrl: ctrl, spinFrame: spinFrame}
}

// Title is empty: the controller's form renders its own title as the first line
// of Content, so the Shell must not draw a second heading (matching Pick).
func (d *formDialog) Title() string { return "" }

// Content renders the scrollable form body — title and fields. The foot row is
// surfaced separately through Footer so the Shell can pin it below the viewport
// (a tall form scrolls its fields but never its hint/confirm row). The form
// sizes itself at construction, so the width is unused.
func (d *formDialog) Content(int) string { return d.ctrl.Body() }

// Footer returns the form's foot row for the Shell to pin below the scrolled
// body, satisfying dialog.Footered. Keeping the hint / submitting / discard-
// confirm row always on screen is the whole point of the split.
func (d *formDialog) Footer() string { return d.ctrl.Foot() }

// Hints returns none: the foot row is delivered through Footer, not the Shell's
// hint slot, so it keeps the form's own multi-state rendering (hints, spinner,
// discard confirm) rather than a flat key list.
func (d *formDialog) Hints() []pkey.Hint { return nil }

// ScrollTo forwards the form's focused-field region so the Shell scrolls to
// follow focus through a create form taller than the height cap. It satisfies
// dialog.ScrollHint.
func (d *formDialog) ScrollTo() (top, height int, ok bool) { return d.ctrl.FocusRegion() }

// Update routes a message into the controller and maps its outcome onto the
// dialog contract. A single-issue submit keeps the dialog on the stack in a
// submitting state and emits formSubmitMsg; a bulk submit closes the dialog and
// lets the section park its confirmation. formFinishMsg resolves an in-flight
// submit — close on success, inline error on failure.
func (d *formDialog) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	if fin, ok := msg.(formFinishMsg); ok {
		if fin.err == nil {
			return d, nil, dialog.ResultClose
		}
		d.ctrl.SetError("✗ " + fin.err.Error())
		return d, nil, dialog.ResultNone
	}
	if set, ok := msg.(formSetTypesMsg); ok {
		d.ctrl.SetTypeOptions(set.types)
		return d, nil, dialog.ResultNone
	}
	cmd, outcome := d.ctrl.Update(msg)
	switch outcome {
	case action.OutcomeChanged:
		// The project pill moved: ask the section to refetch its types. The form
		// stays open with the old list until the new one lands.
		return d, tea.Batch(cmd, emitFormProjectChanged(d.ctrl.Project())), dialog.ResultNone
	case action.OutcomeSubmit:
		req, ok := d.ctrl.Request()
		if !ok {
			return d, cmd, dialog.ResultNone
		}
		// Stay on the stack either way and let the section resolve the request:
		// a single-issue write enters the submitting state and closes on
		// formFinishMsg; a bulk collect is swapped for its y/N confirm in the
		// same Update (closeForm then park), so the overlay never blinks empty.
		if !req.Mode.Bulk() {
			d.ctrl.SetSubmitting(d.spinFrame)
		}
		return d, tea.Batch(cmd, emitFormSubmit(req)), dialog.ResultNone
	case action.OutcomeEditor:
		// Hand the draft to $EDITOR; the dialog stays open so a failed launch
		// loses nothing — handleEditor closes it once the round-trip resolves.
		return d, tea.Batch(cmd, input.Edit("comment:"+d.ctrl.IssueKey(), d.ctrl.Draft())), dialog.ResultNone
	case action.OutcomeCancel:
		// The controller already reset itself on the confirmed discard; drop it.
		return d, cmd, dialog.ResultClose
	case action.OutcomeNone:
	}
	return d, cmd, dialog.ResultNone
}

// emitFormSubmit defers the request to the section's Update on the next loop.
func emitFormSubmit(req action.Request) tea.Cmd {
	return func() tea.Msg { return formSubmitMsg{req: req} }
}

// emitFormProjectChanged defers the project-changed signal to the section's
// Update on the next loop.
func emitFormProjectChanged(project string) tea.Cmd {
	return func() tea.Msg { return formProjectChangedMsg{project: project} }
}
