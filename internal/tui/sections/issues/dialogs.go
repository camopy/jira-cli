// The results list's modal overlay stack: the section-owned dialog.Stack that
// frames the facet, jumplist, transition, preset, and bulk-confirm overlays.
// Each pick or confirm is pushed with the action its acceptance triggers bound
// at push time (sectionPick/sectionConfirm), so the generic stack needs no
// side discriminator and a popped dialog carries everything it needs to act.

package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gechr/primer/scrollbar"

	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// sectionPick binds a Pick to the action its accepted selection runs. The
// closure captures whatever the outcome needs (an issue key, the facet table),
// so no pending state has to be stashed beside the stack and kept in step
// with it — stacked dialogs each carry their own continuation by construction.
type sectionPick struct {
	*dialog.Pick
	onSubmit func(sel picker.Item) tea.Cmd
}

// Update keeps the wrapper on the stack: the embedded Pick mutates in place,
// so the wrapper — not the bare Pick the inner Update returns — must come back
// as the stack's dialog, or the bound action would be lost on the first
// keystroke.
func (p sectionPick) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	_, cmd, res := p.Pick.Update(msg)
	return p, cmd, res
}

// sectionConfirm likewise binds a Confirm to the action an approval runs.
type sectionConfirm struct {
	*dialog.Confirm
	onConfirm func() tea.Cmd
}

// Update keeps the wrapper on the stack — see [sectionPick.Update].
func (c sectionConfirm) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	_, cmd, res := c.Confirm.Update(msg)
	return c, cmd, res
}

// pushPick opens a pick dialog whose accepted selection runs onSubmit.
func (r *results) pushPick(title string, items []picker.Item, onSubmit func(sel picker.Item) tea.Cmd) {
	r.dialogs.Push(sectionPick{Pick: dialog.NewPick(title, items), onSubmit: onSubmit})
}

// pushPickWithGrace opens a pick that arrived on an async result (a fetch
// landing) rather than on a keypress, so it briefly absorbs in-flight
// keystrokes — see [dialog.Stack.PushWithGrace]. Every graced pick shares the
// sectionPick concrete type, so any two count as the same kind for the reopen
// exemption; revisit that granularity before gracing a pick whose misfire
// would be more than a filter keystroke.
func (r *results) pushPickWithGrace(title string, items []picker.Item, onSubmit func(sel picker.Item) tea.Cmd) {
	r.dialogs.PushWithGrace(sectionPick{Pick: dialog.NewPick(title, items), onSubmit: onSubmit})
}

// pushConfirm opens a y/N confirmation whose approval runs onConfirm.
func (r *results) pushConfirm(prompt string, onConfirm func() tea.Cmd) {
	r.dialogs.Push(sectionConfirm{Confirm: dialog.NewConfirm(prompt), onConfirm: onConfirm})
}

// newSectionShell builds the frame the section's dialog stack draws around its
// box-less pick and confirm dialogs — the shared core recipe with the section
// overlays' historical wider edge margin (overlayMargin) and unthemed
// scrollbar. It is rebuilt on restyle so an open dialog never renders with
// stale chrome.
func newSectionShell(styles core.Styles) dialog.Shell {
	return core.NewDialogShell(styles, overlayMargin, scrollbar.Styles{})
}

// updateDialog routes a message into the open section dialog and runs the
// action a resolved one bound at push time. On ResultNone the dialog stays
// open and its command flows through; a close drops the dialog and nothing
// else happens; a submit runs the bound action with the accepted payload.
func (r *results) updateDialog(msg tea.Msg) tea.Cmd {
	cmd, popped, res := r.dialogs.Update(msg)
	if res != dialog.ResultSubmit {
		return cmd
	}
	switch d := popped.(type) {
	case sectionPick:
		if sel, ok := d.Selected(); ok {
			return d.onSubmit(sel)
		}
	case sectionConfirm:
		// A Confirm only ever submits accepted (a decline closes), so the
		// bound action runs unconditionally here.
		return d.onConfirm()
	}
	return cmd
}
