package jql

import (
	"strings"
	"testing"
)

// The --updated/--created/--resolved flags compile to JQL date clauses. A bare
// value is a lower bound (>=); a comparator prefix sets the operator; a ".."
// range gives two inclusive bounds (open-ended on either side); relative
// durations pass through unquoted and must carry a sign, while absolute dates
// are quoted as Jira requires.
func TestBuildOptionsDateFilters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		opts    BuildOptions
		want    string
		wantErr bool
	}{
		{"relative lower bound", BuildOptions{Updated: "-7d"}, "updated >= -7d", false},
		{"relative weeks", BuildOptions{Updated: "-2w"}, "updated >= -2w", false},
		{"positive relative is accepted", BuildOptions{Created: "+2w"}, "created >= +2w", false},
		{"multiple range delimiters are rejected", BuildOptions{Created: "2026-01-01..2026-02-01..2026-03-01"}, "", true},
		{"absolute lower bound is quoted", BuildOptions{Created: "2026-01-01"}, `created >= "2026-01-01"`, false},
		{"explicit less-than", BuildOptions{Resolved: "<2026-02-01"}, `resolved < "2026-02-01"`, false},
		{"explicit gte on relative", BuildOptions{Updated: ">=-2w"}, "updated >= -2w", false},
		{"explicit lte absolute", BuildOptions{Resolved: "<=2026-02-01"}, `resolved <= "2026-02-01"`, false},
		{"inclusive range", BuildOptions{Created: "2026-01-01..2026-02-01"}, `created >= "2026-01-01" AND created <= "2026-02-01"`, false},
		{"open upper bound", BuildOptions{Created: "2026-01-01.."}, `created >= "2026-01-01"`, false},
		{"open lower bound", BuildOptions{Created: "..2026-02-01"}, `created <= "2026-02-01"`, false},
		{"unsigned relative is rejected", BuildOptions{Updated: "7d"}, "", true},
		{"garbage is rejected", BuildOptions{Created: "yesterday"}, "", true},
		{"colon is not a range delimiter", BuildOptions{Created: "2026-01-01:2026-02-01"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.opts.Build()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got JQL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("JQL missing %q:\n%s", tc.want, got)
			}
		})
	}
}
