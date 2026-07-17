package dialog

import (
	tea "charm.land/bubbletea/v2"

	pkey "github.com/gechr/primer/key"
)

// Result is what a dialog's Update reports back to the Stack: whether the
// dialog stays open, or is finished and should be popped. Submit and Close are
// distinct so a caller reading the popped dialog can tell an accepted result
// from an abandoned one without inspecting the payload.
type Result int

const (
	// ResultNone means the dialog consumed (or ignored) the message and stays
	// open.
	ResultNone Result = iota
	// ResultSubmit means the dialog completed successfully; the Stack pops it
	// and returns it so the caller can read a typed payload.
	ResultSubmit
	// ResultClose means the dialog was dismissed without a result; the Stack
	// pops it just the same, and the caller distinguishes the two by Result.
	ResultClose
)

// Dialog is one overlay's content, free of any chrome. Implementations render
// their body into the width the Shell allots and never draw their own border,
// title bar, or scrollbar — the Shell owns all of that. The interface is
// deliberately minimal: async and busy state belong to the concrete dialog and
// its own messages, not to this contract.
type Dialog interface {
	// Title is the heading the Shell renders above the body; "" omits it.
	Title() string
	// Update routes a message into the dialog. It returns the (possibly
	// updated) dialog so value-typed implementations compose cleanly, a command
	// to run, and the Result telling the Stack whether to keep or pop it.
	Update(msg tea.Msg) (Dialog, tea.Cmd, Result)
	// Content renders the body — and only the body — to fit within width
	// columns. The Shell frames, scrolls, and centers whatever it returns.
	Content(width int) string
	// Hints are the key bindings the Shell renders as the dialog's foot row.
	Hints() []pkey.Hint
}

// DismissResult is the shared Update rule for momentary dialogs (the help
// sheet, the activity log): the first key press or mouse click closes them,
// every other message leaves them open.
func DismissResult(msg tea.Msg) Result {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg:
		return ResultClose
	default:
		return ResultNone
	}
}

// Footered is an optional Dialog capability. A dialog that wants a row pinned
// below its scrollable body — a form's hint/confirm/submitting row that must
// stay visible however the body scrolls — returns that row here. The Shell
// scrolls only Content and always renders Footer beneath the viewport (inside
// the same box). A dialog that does not implement this frames through Content
// alone, as before. Pairs with [ScrollHint], whose region is measured against
// Content, not the footer.
type Footered interface {
	Footer() string
}

// ScrollHint is an optional Dialog capability. A dialog whose body can grow
// taller than the Shell's height cap reports the body region it wants kept on
// screen — a form's focused field, say — so the Shell scrolls to follow it as
// focus moves rather than stranding the cursor below the fold. top is the
// region's first body line (0-based, excluding any Shell-drawn title); height
// is its line span. A dialog that does not implement this, or returns ok false,
// leaves the viewport at the top (the default before this seam existed).
type ScrollHint interface {
	ScrollTo() (top, height int, ok bool)
}

// SelfFramed is an optional Dialog capability. A dialog that draws its own
// complete frame — border, title, and sizing — reports true, and the Stack
// places its Content verbatim over the backdrop instead of framing and
// scrolling it through the Shell (which would clip and interleave a pre-drawn
// box). A dialog that does not implement this, or returns false, gets the
// Shell's chrome as usual.
type SelfFramed interface {
	SelfFramed() bool
}
