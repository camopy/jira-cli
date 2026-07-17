// Package titlebox draws a rounded box with its title embedded in the top
// border — "╭ title ─────╮" — around a body the caller has already rendered.
// It is the framing primitive the form's fields use, kept apart so any overlay
// wanting a labeled panel can reuse it without depending on the form.
//
// The widget is style-agnostic: the border and title colors arrive through
// [Styles], so a caller signals focus by handing it a different pair. It never
// truncates the body — a caller sizes its content to width minus [Chrome] and
// the box pads each line out to the interior.
package titlebox
