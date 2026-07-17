// Package pill renders a labeled cycle selector — "type       ‹ Task ›" — a
// compact one-of chooser for a form field that needs no text area. It is the
// counterpart to titlebox: where a box frames somewhere to type, a pill frames
// a value stepped through a fixed set, so the two read as distinct affordances.
//
// Like titlebox it is style-agnostic: the label and chevron colors arrive
// through [Styles], so a caller signals focus by handing it a brighter pair.
package pill
