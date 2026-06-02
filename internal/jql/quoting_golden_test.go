package jql

import (
	"strings"
	"testing"
)

// Every filter value reaches JQL through the one quoter, Value: a safe
// identifier (letters, digits, _ - .) is emitted bare; anything else (spaces,
// @, punctuation) is double-quoted with embedded quotes escaped. This golden
// matrix locks that single rule across every field that composes a clause, so
// a regression in Value/isSafeJQLIdentifier — which would either break
// queries (unquoted spaces) or over-quote — fails loudly. It drives the real
// Build() path and asserts the produced clause fragment.
func TestQuotingInvariantAcrossFilters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts BuildOptions
		want string
	}{
		{"safe project is bare", BuildOptions{Projects: []string{"ENG"}}, "project = ENG"},
		{"project with space is quoted", BuildOptions{Projects: []string{"My Project"}}, `project = "My Project"`},
		{"safe key (hyphen) is bare", BuildOptions{Keys: []string{"ENG-1"}}, "key = ENG-1"},
		{"single label bare", BuildOptions{Labels: []string{"bug"}}, "labels = bug"},
		{"label with space quoted", BuildOptions{Labels: []string{"needs triage"}}, `labels = "needs triage"`},
		{"multi-value IN quotes per member", BuildOptions{Labels: []string{"a", "b c"}}, `labels in (a, "b c")`},
		{"issue type with hyphen is bare", BuildOptions{IssueTypes: []string{"Sub-task"}}, "issuetype = Sub-task"},
		{"priority bare", BuildOptions{Priorities: []string{"High"}}, "priority = High"},
		{"status with space quoted", BuildOptions{Statuses: []string{"In Review"}}, `status = "In Review"`},
		{"embedded quote escaped", BuildOptions{Statuses: []string{`Say "hi"`}}, `status = "Say \"hi\""`},
		{"assignee me is a function, not quoted", BuildOptions{Assignee: "me"}, "assignee = currentUser()"},
		{"assignee email is quoted (unsafe @)", BuildOptions{Assignee: "a@b.com"}, `assignee = "a@b.com"`},
		{"reporter none is is-EMPTY", BuildOptions{Reporter: "none"}, "reporter is EMPTY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.opts.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("Build() = %q\n  missing quoted clause %q", got, tc.want)
			}
		})
	}
}
