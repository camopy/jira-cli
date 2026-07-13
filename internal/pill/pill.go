package pill

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	fillGreen  = lipgloss.Color("#1f845a")
	fillYellow = lipgloss.Color("#e2b203")
	fillBlue   = lipgloss.Color("#1d7afc")
	fillGray   = lipgloss.Color("#626f86")
	// fillFallbacks color statuses with no recognized category: a per-name
	// hash picks one, keeping distinct statuses distinguishable without ever
	// touching a remappable palette slot.
	fillFallbacks = []color.Color{
		lipgloss.Color("#8f7ee7"), // purple
		lipgloss.Color("#e56910"), // orange
		lipgloss.Color("#227d9b"), // teal
		lipgloss.Color("#da62ac"), // magenta
		lipgloss.Color("#82b536"), // lime
	}
)

// Fill picks the pill background. Jira's own color designation
// (statusCategory.colorName) is preferred so the badge matches the Jira UI;
// when it is absent or unrecognized the stable category key is used; failing
// both, a per-name hash keeps distinct statuses distinguishable. Every result
// is a fixed truecolor fill — see the package doc for why theme-tracking was
// dropped.
func Fill(status, category, colorName string) color.Color {
	switch normalizeKey(colorName) {
	case "green":
		return fillGreen
	case "yellow":
		return fillYellow
	case "blue-gray", "blue-grey", "blue":
		return fillBlue
	case "medium-gray", "medium-grey", "gray", "grey":
		return fillGray
	}
	switch normalizeKey(category) {
	case "done":
		return fillGreen
	case "indeterminate":
		return fillYellow
	case "new":
		return fillBlue
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte("status:" + normalizeKey(status)))
	return fillFallbacks[h.Sum32()%uint32(len(fillFallbacks))]
}

var (
	textDark  = lipgloss.Color("#1c1c1c")
	textLight = lipgloss.Color("#f5f5f5")
)

// Foreground picks near-black or near-white text for legibility on the pill
// fill, judged by the fill's Rec. 601 luma. Every fill is a fixed truecolor
// value, so the choice is deterministic.
func Foreground(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA() // channels 0..0xffff, alpha-premultiplied
	if (299*r+587*g+114*b)/1000 > 0x7fff {
		return textDark
	}
	return textLight
}

// Style is the badge style for a status: the category-derived fill, a
// contrasting foreground, bold — so the pill reads on any terminal background.
func Style(status, category, colorName string) lipgloss.Style {
	bg := Fill(status, category, colorName)
	return lipgloss.NewStyle().Background(bg).Foreground(Foreground(bg)).Bold(true)
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
