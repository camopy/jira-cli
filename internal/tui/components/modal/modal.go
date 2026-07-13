package modal

import (
	lipgloss "charm.land/lipgloss/v2"

	"github.com/gechr/primer/overlay"
)

// Frame is one overlay's geometry: the injected box style, a width cap, and
// the margin kept clear at the screen edges.
type Frame struct {
	// Box draws the border and padding; inject the app's overlay style.
	Box lipgloss.Style
	// MaxWidth caps the box's total width in columns; 0 means no cap beyond
	// the margin. The cap keeps a long prefill (a full summary, a pasted
	// line) from bleeding past the screen edges.
	MaxWidth int
	// Margin is how many columns stay clear at the screen edges when the
	// screen, not MaxWidth, is the binding constraint.
	Margin int
}

// Place boxes content and composites it centered over backdrop, which must
// already be screenWidth columns and height rows. The box width is the
// screen minus the margin, capped at MaxWidth and floored at one column so a
// tiny terminal can never push a negative width into lipgloss.
func (f Frame) Place(backdrop, content string, screenWidth, height int) string {
	boxW := screenWidth - f.Margin
	if f.MaxWidth > 0 && boxW > f.MaxWidth {
		boxW = f.MaxWidth
	}
	if boxW < 1 {
		boxW = 1
	}
	box := f.Box.Width(boxW).Render(content)
	return overlay.Place(backdrop, box, screenWidth, height, overlay.Center)
}
