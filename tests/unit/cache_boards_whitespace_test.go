// Cache write strips leading and trailing whitespace from board names;
// preserves internal whitespace and Unicode characters verbatim.
//
// The cache prime path normalizes board names before writing. This unit
// test pins the contract via the exported helper `jira.NormalizeBoardName`,
// which the prime path calls per board.
package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestNormalizeBoardNameTrimsLeadingAndTrailingWhitespace(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Engineering Sprint  ", "Engineering Sprint"},
		{"\tEngineering Sprint\n", "Engineering Sprint"},
		{"Engineering Sprint", "Engineering Sprint"}, // no-op
	}
	for _, c := range cases {
		got := jira.NormalizeBoardName(c.in)
		if got != c.want {
			t.Errorf("NormalizeBoardName(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeBoardNamePreservesInternalWhitespace(t *testing.T) {
	in := "Hello\tWorld"
	if got := jira.NormalizeBoardName(in); got != in {
		t.Fatalf("internal tab lost: NormalizeBoardName(%q) = %q", in, got)
	}
	in = "Multi  Space  Name"
	if got := jira.NormalizeBoardName(in); got != in {
		t.Fatalf("internal multi-space collapsed: NormalizeBoardName(%q) = %q", in, got)
	}
}

func TestNormalizeBoardNamePreservesUnicode(t *testing.T) {
	in := "Café & Croissant 🥐"
	if got := jira.NormalizeBoardName(in); got != in {
		t.Fatalf("Unicode lost: NormalizeBoardName(%q) = %q", in, got)
	}
	// Leading/trailing whitespace around Unicode still trimmed.
	if got := jira.NormalizeBoardName("  Café 🥐  "); got != "Café 🥐" {
		t.Fatalf("Unicode trim wrong: got %q", got)
	}
}
