package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gechr/primer/helpsheet"
	pkey "github.com/gechr/primer/key"
	"github.com/gechr/primer/scrollbar"

	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
)

// helpDialog is the full-keymap sheet as a stack dialog. It carries the merged
// key/description pairs and closes on any key or click, preserving the sheet's
// long-standing dismissal rule now that it rides the dialog stack.
type helpDialog struct {
	pairs   []helpsheet.Pair
	dismiss string
	styles  helpsheet.Styles
}

// Title omits the heading: the sheet's own dismiss line is its only caption.
func (d helpDialog) Title() string { return "" }

// Update closes the sheet on the first key or mouse click and ignores every
// other message, matching the any-key/any-click rule it has always had.
func (d helpDialog) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	return d, nil, dialog.DismissResult(msg)
}

// Content renders the sheet; it draws its own box (see newHelpDialog), so the
// Shell only places it.
func (d helpDialog) Content(width int) string {
	return helpsheet.Model{Pairs: d.pairs, Dismiss: d.dismiss, Styles: d.styles}.Render()
}

// Hints are none: the sheet carries its own dismiss line, not a foot row.
func (d helpDialog) Hints() []pkey.Hint { return nil }

// SelfFramed reports that the sheet draws its own box (see newHelpDialog), so
// the Stack places it verbatim instead of framing and scrolling it — which
// would clip the pre-drawn box on a short terminal.
func (d helpDialog) SelfFramed() bool { return true }

// NewDialogShell builds the frame a dialog stack draws around a box-less
// dialog, from the current theme styles. The 66-column cap keeps every modal
// at a readable measure (pairing with the action controller's 60-column inner
// text width); margin and scrollbar styling are the owner's — the App's stack
// hugs the screen edges, the section stacks keep their historical wider
// margin. Owners rebuild the shell whenever the styles re-derive (theme
// preview or config reload) so an open dialog never renders with stale chrome.
func NewDialogShell(styles Styles, margin int, bar scrollbar.Styles) dialog.Shell {
	return dialog.NewShell(dialog.ShellConfig{
		Styles: dialog.Styles{
			Box:       styles.Overlay,
			Title:     styles.Header,
			HintKey:   styles.HintKey,
			HintText:  styles.HintDesc,
			Scrollbar: bar,
		},
		MaxWidth:       66,
		WidthFraction:  0.9,
		HeightFraction: 0.9,
		Margin:         margin,
		Scrollbar:      scrollbar.Config{TrackSymbol: "│"},
	})
}

// newDialogShell is the App stack's shell. Its scrolling body path (viewport +
// scrollbar) is exercised by the create form and the command palette, both of
// which pin a foot row and scroll their body on a short terminal; the
// scrollbar therefore carries real theme colors — an accent thumb over a dim
// track — so a truncated list reads as scrollable rather than as a stray glyph.
func newDialogShell(styles Styles) dialog.Shell {
	return NewDialogShell(styles, 2, scrollbar.Styles{
		Thumb: styles.HintKey,
		Track: styles.FooterRule,
	})
}
