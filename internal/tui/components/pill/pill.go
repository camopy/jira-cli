package pill

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// Styles are the pill's injected render styles, so the widget stays
// theme-agnostic: a caller signals focus by handing it a brighter pair.
type Styles struct {
	// Label styles the field name to the left of the value.
	Label lipgloss.Style
	// Chevron styles the "‹" "›" markers that frame the value.
	Chevron lipgloss.Style
}

// Render draws "label<pad>‹ value ›". label is padded on the right to labelW
// columns so a column of pills aligns their values; a label already at or past
// labelW is left as is. The value carries no styling of its own — the caller
// styles it before passing it in if it wants to.
func Render(label, value string, labelW int, s Styles) string {
	if pad := labelW - lipgloss.Width(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return s.Label.Render(label) + s.Chevron.Render("‹ ") + value + s.Chevron.Render(" ›")
}
