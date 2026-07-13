package modal

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func backdrop(w, h int) string {
	row := strings.Repeat(".", w)
	return strings.TrimRight(strings.Repeat(row+"\n", h), "\n")
}

func boxed() lipgloss.Style { return lipgloss.NewStyle().Border(lipgloss.NormalBorder()) }

func TestPlaceCapsBoxWidth(t *testing.T) {
	f := Frame{Box: boxed(), MaxWidth: 20, Margin: 6}
	out := f.Place(backdrop(80, 9), "short", 80, 9)
	for line := range strings.SplitSeq(out, "\n") {
		if w := lipgloss.Width(line); w != 80 {
			t.Fatalf("row width %d, want the backdrop's 80", w)
		}
	}
	// The box (border glyphs) must span MaxWidth, not screen-minus-margin.
	var boxRow string
	for line := range strings.SplitSeq(ansi.Strip(out), "\n") {
		if strings.Contains(line, "─") {
			boxRow = line
			break
		}
	}
	if boxRow == "" {
		t.Fatal("no box border found in output")
	}
	first := strings.Index(boxRow, "┌")
	last := strings.Index(boxRow, "┐")
	if first < 0 || last < 0 {
		t.Fatalf("border corners missing: %q", boxRow)
	}
	if got := lipgloss.Width(boxRow[first:last]) + 1; got != 20 {
		t.Errorf("box spans %d columns, want the 20-column cap", got)
	}
}

func TestPlaceUsesMarginWhenScreenBinds(t *testing.T) {
	f := Frame{Box: boxed(), MaxWidth: 66, Margin: 6}
	out := ansi.Strip(f.Place(backdrop(30, 9), "x", 30, 9))
	for line := range strings.SplitSeq(out, "\n") {
		if first := strings.Index(line, "┌"); first >= 0 {
			last := strings.Index(line, "┐")
			if got := lipgloss.Width(line[first:last]) + 1; got != 30-6 {
				t.Errorf("box spans %d columns, want screen minus margin = 24", got)
			}
			return
		}
	}
	t.Fatal("no box border found")
}

func TestPlaceTinyScreenClampsToOneColumn(t *testing.T) {
	f := Frame{Box: lipgloss.NewStyle(), MaxWidth: 66, Margin: 6}
	// Margin exceeds the screen: the width must clamp, not go negative (a
	// negative lipgloss width panics).
	out := f.Place(backdrop(4, 3), "x", 4, 3)
	if out == "" {
		t.Fatal("empty output on a tiny screen")
	}
}

func TestPlaceKeepsBackdropAroundBox(t *testing.T) {
	f := Frame{Box: boxed(), MaxWidth: 10, Margin: 2}
	out := ansi.Strip(f.Place(backdrop(40, 7), "hi", 40, 7))
	rows := strings.Split(out, "\n")
	if !strings.HasPrefix(rows[0], ".") {
		t.Errorf("backdrop missing above the box: %q", rows[0])
	}
	if !strings.Contains(out, "hi") {
		t.Error("content missing from composite")
	}
}
