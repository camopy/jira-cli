package components

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// resetBg restores the terminal's default background.
const resetBg = "\x1b[49m"

// PersistBgFull right-pads `line` to `width` then applies PersistBg so the
// background spans the full terminal row even after wrapping.
func PersistBgFull(line, bg string, width int) string {
	w := lipgloss.Width(line)
	if w < width {
		line += strings.Repeat(" ", width-w)
	}
	return PersistBg(line, bg)
}

// PersistBg re-emits the bg escape after every inner SGR so foreground
// color changes don't blow away the row background. This is the only way
// to reliably highlight a row that contains colored cells.
func PersistBg(line, bg string) string {
	if bg == "" {
		return line
	}
	var b strings.Builder
	b.WriteString(bg)
	for i := 0; i < len(line); i++ {
		if line[i] == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && ((line[j] >= '0' && line[j] <= '9') || line[j] == ';') {
				j++
			}
			if j < len(line) && line[j] == 'm' {
				j++
				b.WriteString(line[i:j])
				b.WriteString(bg)
				i = j - 1
				continue
			}
		}
		b.WriteByte(line[i])
	}
	b.WriteString(resetBg)
	return b.String()
}

// ColorToANSIBg converts a color to an ANSI 24-bit bg escape.
func ColorToANSIBg(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}
