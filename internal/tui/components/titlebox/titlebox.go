package titlebox

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

// hPad is the padding columns inside the border on each side of the body.
const hPad = 1

// Chrome is the columns a titled box spends on its border and padding. A caller
// sizes the body it renders to the box width minus Chrome, so its content fills
// the interior exactly without overrunning the frame.
const Chrome = 2 + 2*hPad

// Styles are the box's injected render styles, so the widget stays
// theme-agnostic: a caller signals focus by handing it a brighter pair.
type Styles struct {
	// Border styles the box's border glyphs.
	Border lipgloss.Style
	// Title styles the label embedded in the top border.
	Title lipgloss.Style
}

// Render draws body inside a rounded box with title embedded in the top border
// ("╭ title ─────╮"). width is the box's total column count (border included);
// body is padded — never truncated — to the interior, so size it to
// width-[Chrome]. A blank title, or one too wide for the interior, draws a
// plain top edge. Multi-line bodies box every line.
func Render(title, body string, width int, s Styles) string {
	innerW := max(1, width-2)
	bd := lipgloss.RoundedBorder()

	var top strings.Builder
	top.WriteString(s.Border.Render(bd.TopLeft))
	used := 0
	if title != "" {
		t := " " + title + " "
		if w := lipgloss.Width(t); w <= innerW {
			top.WriteString(s.Title.Render(t))
			used = w
		}
	}
	if dashes := innerW - used; dashes > 0 {
		top.WriteString(s.Border.Render(strings.Repeat(bd.Top, dashes)))
	}
	top.WriteString(s.Border.Render(bd.TopRight))

	left, right := s.Border.Render(bd.Left), s.Border.Render(bd.Right)
	pad := strings.Repeat(" ", hPad)
	contentW := max(1, innerW-2*hPad)

	var b strings.Builder
	b.WriteString(top.String())
	b.WriteString("\n")
	for ln := range strings.SplitSeq(body, "\n") {
		// Clip a line wider than the interior so a mis-sized body can never push
		// the right border out and jag the frame; a fitting line is padded out.
		if lipgloss.Width(ln) > contentW {
			ln = ansi.Truncate(ln, contentW, "")
		}
		fill := max(0, contentW-lipgloss.Width(ln))
		b.WriteString(left)
		b.WriteString(pad)
		b.WriteString(ln)
		b.WriteString(strings.Repeat(" ", fill))
		b.WriteString(pad)
		b.WriteString(right)
		b.WriteString("\n")
	}
	b.WriteString(s.Border.Render(bd.BottomLeft + strings.Repeat(bd.Bottom, innerW) + bd.BottomRight))
	return b.String()
}
