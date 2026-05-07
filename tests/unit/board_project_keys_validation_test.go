// Project keys are interpolated raw into the JQL `project in (...)`
// clause. Any key carrying whitespace, parentheses, commas, quotes, or
// newlines would silently corrupt the emitted JQL — Atlassian's API
// constrains keys to `[A-Z][A-Z0-9_]*` server-side, so this is a
// defense-in-depth filter for malformed wire data, not a substitute
// for upstream validation.
package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestSanitizeProjectKeysDropsJQLMetacharacters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "all valid keys round-trip untouched",
			in:   []string{"ENG", "PLAT", "OPS_2"},
			want: []string{"ENG", "PLAT", "OPS_2"},
		},
		{
			name: "drops keys with whitespace, comma, parens, quotes, newline",
			in:   []string{"OK", "BAD KEY", "FOO,BAR", "OOPS)", "(BAD", "WITH\nLF", "WITH\"QUOTE"},
			want: []string{"OK"},
		},
		{
			name: "drops empty key",
			in:   []string{"", "ENG"},
			want: []string{"ENG"},
		},
		{
			name: "preserves order of valid keys",
			in:   []string{"C", "A", "B"},
			want: []string{"C", "A", "B"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := jira.SanitizeProjectKeys(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len=%d; want %d (got=%v want=%v)", len(got), len(c.want), got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("[%d] = %q; want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestSanitizeProjectKeysReturnsDroppedCount(t *testing.T) {
	t.Parallel()

	in := []string{"GOOD", "BAD KEY", "ALSO,BAD", ""}
	_, dropped := jira.SanitizeProjectKeys(in)
	if dropped != 3 {
		t.Errorf("dropped=%d; want 3", dropped)
	}
}
