// Package dialog is a domain-free stack of modal overlays for a Bubble Tea
// application. A Dialog contributes only its parts — a title, a body, and a
// row of key hints — and stays ignorant of borders, centering, scrolling, and
// screen size. A Shell supplies that chrome: it caps a dialog to a fraction of
// the screen and to an absolute maximum, scrolls the body internally when it
// overflows the cap, and centers the framed box over a backdrop. A Stack owns
// the ordered dialogs (top is last) plus one Shell, routes every message to
// the top dialog, and pops it when the dialog reports it is done — handing the
// popped Dialog back so the caller can read a typed payload off it.
//
// The core — the [Dialog] interface, [Stack], [Shell], and the generic
// [Confirm] — imports only the Charm stack and the primer primitives it builds
// on; styles are injected through [Styles], so that core is liftable to another
// repository or upstreamable to primer without change. The convenience [Pick]
// dialog is the one exception: it wraps the application's picker component
// (which carries its own styling), so lifting Pick means taking that component
// along, or re-writing it against the target app's picker.
package dialog
