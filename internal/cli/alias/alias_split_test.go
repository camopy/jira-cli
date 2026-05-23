package alias

import (
	"slices"
	"testing"
)

// Alias expansions are stored as a single string and re-split into argv
// on use. The splitter must follow POSIX shell grammar so a quoted JQL
// clause survives a set→list→expand round trip. In particular a
// backslash inside single quotes is a literal backslash, not an escape.
func TestSplitAliasExpansionShellGrammar(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			"double-quoted clause with spaces",
			`issue list --jql "project = PROJ"`,
			[]string{"issue", "list", "--jql", "project = PROJ"},
		},
		{
			"single-quoted backslash is literal",
			`search jql 'path\to\thing'`,
			[]string{"search", "jql", `path\to\thing`},
		},
		{
			"escaped space outside quotes",
			`issue list --jql project\ =\ PROJ`,
			[]string{"issue", "list", "--jql", "project = PROJ"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitAliasExpansion(tc.in)
			if err != nil {
				t.Fatalf("splitAliasExpansion(%q) error = %v", tc.in, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("splitAliasExpansion(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
