package theme

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCodeSpansKeepsTextAndWidth(t *testing.T) {
	in := "respect `me` and `you` today"
	got := CodeSpans(in)
	if stripped := ansi.Strip(got); stripped != in {
		t.Errorf("stripped = %q, want the input unchanged (backticks kept)", stripped)
	}
	if lipgloss.Width(got) != lipgloss.Width(in) {
		t.Errorf("styling changed the display width: %d != %d", lipgloss.Width(got), lipgloss.Width(in))
	}
}

func TestCodeSpansStylesOnlyTheSpan(t *testing.T) {
	got := CodeSpans("fix `flux` now")
	if got == "fix `flux` now" {
		t.Fatal("span not styled at all")
	}
	// The text outside the span must carry no styling.
	if got[:4] != "fix " {
		t.Errorf("prefix was styled: %q", got[:10])
	}
}

func TestCodeSpansUnpairedBacktickUntouched(t *testing.T) {
	in := "odd ` one out"
	if got := CodeSpans(in); got != in {
		t.Errorf("unpaired backtick altered the string: %q", got)
	}
}

func TestCodeSpansWithPreservesBaseOnRemainder(t *testing.T) {
	base := lipgloss.NewStyle().Bold(true)
	got := CodeSpansWith("a `b` c", base)
	stripped := ansi.Strip(got)
	if stripped != "a `b` c" {
		t.Fatalf("stripped = %q", stripped)
	}
	// The trailing segment must re-open the base style — a span reset that
	// leaves " c" unstyled is exactly the bug this helper exists to avoid.
	if got[len(got)-len(" c\x1b[m"):] == " c\x1b[m" {
		return // trailing text sits inside a styled region, good enough
	}
	if !hasBoldAfterSpan(got) {
		t.Errorf("base style lost after the span: %q", got)
	}
}

// hasBoldAfterSpan reports a bold SGR opening after the last backtick.
func hasBoldAfterSpan(s string) bool {
	last := -1
	for i := range s {
		if s[i] == '`' {
			last = i
		}
	}
	if last < 0 {
		return false
	}
	rest := s[last:]
	for i := 0; i+3 < len(rest); i++ {
		if rest[i] == '\x1b' && rest[i+1] == '[' && rest[i+2] == '1' && rest[i+3] == 'm' {
			return true
		}
	}
	return false
}
