package titlebox

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

// lines strips styling and splits a rendered box into rows for structural
// assertions.
func lines(s string) []string { return strings.Split(ansi.Strip(s), "\n") }

func TestRenderEmbedsTitleInTopBorder(t *testing.T) {
	got := lines(Render("summary", "hi", 20, Styles{}))
	if len(got) != 3 {
		t.Fatalf("a single-line body should box to 3 rows, got %d:\n%s", len(got), strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(got[0], "╭ summary ") || !strings.HasSuffix(got[0], "╮") {
		t.Errorf("top border missing embedded title: %q", got[0])
	}
	if !strings.Contains(got[1], "hi") || !strings.HasPrefix(got[1], "│") || !strings.HasSuffix(got[1], "│") {
		t.Errorf("body row not framed: %q", got[1])
	}
	if !strings.HasPrefix(got[2], "╰") || !strings.HasSuffix(got[2], "╯") {
		t.Errorf("bottom border malformed: %q", got[2])
	}
}

func TestRenderPadsEveryRowToWidth(t *testing.T) {
	const width = 24
	for _, row := range lines(Render("t", "a\nbb\nccc", width, Styles{})) {
		if w := lipgloss.Width(row); w != width {
			t.Errorf("row %q width = %d, want %d", row, w, width)
		}
	}
}

func TestRenderBlankTitleDrawsPlainEdge(t *testing.T) {
	top := lines(Render("", "x", 12, Styles{}))[0]
	if strings.ContainsAny(top, "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("blank title should leave a plain top edge: %q", top)
	}
	if !strings.HasPrefix(top, "╭─") {
		t.Errorf("plain top edge should be all border glyphs: %q", top)
	}
}

func TestRenderOverwideTitleFallsBackToPlainEdge(t *testing.T) {
	// A title wider than the interior cannot embed; the box keeps its frame
	// rather than overflowing the width.
	top := lines(Render("a very long field label", "x", 10, Styles{}))[0]
	if lipgloss.Width(top) != 10 {
		t.Errorf("overwide title broke the width: %q (%d)", top, lipgloss.Width(top))
	}
	if strings.Contains(top, "very") {
		t.Errorf("overwide title should not embed: %q", top)
	}
}

func TestRenderMultilineBodyBoxesEachLine(t *testing.T) {
	got := lines(Render("desc", "one\ntwo\nthree", 20, Styles{}))
	if len(got) != 5 { // top + 3 body + bottom
		t.Fatalf("three-line body should box to 5 rows, got %d", len(got))
	}
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(got[1]+got[2]+got[3], want) {
			t.Errorf("body line %q missing", want)
		}
	}
}
