package pill

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderFramesValueWithChevrons(t *testing.T) {
	got := ansi.Strip(Render("type", "Task", 10, Styles{}))
	if !strings.Contains(got, "‹ Task ›") {
		t.Errorf("value not framed by chevrons: %q", got)
	}
	if !strings.HasPrefix(got, "type") {
		t.Errorf("label missing: %q", got)
	}
}

func TestRenderPadsLabelToColumn(t *testing.T) {
	// A short label is padded so its value starts at the same column as a
	// longer label's would.
	short := ansi.Strip(Render("t", "A", 10, Styles{}))
	long := ansi.Strip(Render("project", "A", 10, Styles{}))
	if i, j := strings.Index(short, "‹"), strings.Index(long, "‹"); i != j {
		t.Errorf("values not column-aligned: %q at %d vs %q at %d", short, i, long, j)
	}
}

func TestRenderLeavesOverlongLabelUnpadded(t *testing.T) {
	// A label already at the column width is not truncated or re-padded.
	got := ansi.Strip(Render("a-very-long-label", "V", 4, Styles{}))
	if !strings.HasPrefix(got, "a-very-long-label‹") {
		t.Errorf("overlong label should sit flush against the chevron: %q", got)
	}
	if lipgloss.Width(got) == 0 {
		t.Fatal("empty render")
	}
}
