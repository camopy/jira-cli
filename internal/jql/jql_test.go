package jql_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jql"
)

func TestJQLBuilderBuildsCommonDeveloperFilters(t *testing.T) {
	query, err := jql.BuildOptions{
		Projects:   []string{"PROJ"},
		Keys:       []string{"PROJ-1", "PROJ-2"},
		Assignee:   "me",
		Epics:      []string{"EPIC-1"},
		Statuses:   []string{"In Progress", "To Do"},
		Priorities: []string{"High"},
		Labels:     []string{"backend"},
		OrderBy:    "updated",
		Descending: true,
	}.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := `project = PROJ AND key in (PROJ-1, PROJ-2) AND assignee = currentUser()` +
		` AND parent = EPIC-1 AND status in ("In Progress", "To Do")` +
		` AND priority = High AND labels = backend ORDER BY updated DESC`
	if query != want {
		t.Fatalf("Build() = %q, want %q", query, want)
	}
}

func TestJQLBuilderExpandsIssueKeyRanges(t *testing.T) {
	query, err := (jql.BuildOptions{
		Keys:       []string{"ABC-1..3", "ABC-5:ABC-6", "XYZ-1:2"},
		Descending: true,
	}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := "key in (ABC-1, ABC-2, ABC-3, ABC-5, ABC-6, XYZ-1, XYZ-2) ORDER BY updated DESC"
	if query != want {
		t.Fatalf("Build() = %q, want %q", query, want)
	}
}

func TestJQLBuilderRejectsInvalidIssueKeyRanges(t *testing.T) {
	tests := []string{"ABC-3..1", "ABC-1:XYZ-100"}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := (jql.BuildOptions{Keys: []string{key}}).Build()
			if err == nil {
				t.Fatalf("Build() accepted invalid issue key range %q", key)
			}
		})
	}
}

func TestJQLBuilderDefaultsToBoundedIssueList(t *testing.T) {
	query, err := (jql.BuildOptions{}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if query != jql.DefaultIssueListJQL {
		t.Fatalf("Build() = %q, want %q", query, jql.DefaultIssueListJQL)
	}
	if strings.Contains(query, "currentUser()") {
		t.Fatalf("default query should not imply --mine: %q", query)
	}
}

func TestJQLBuilderAppliesSortWithoutAdditionalFilters(t *testing.T) {
	query, err := (jql.BuildOptions{OrderBy: "status", Descending: true}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := "updated >= -365d ORDER BY status DESC"
	if query != want {
		t.Fatalf("Build() = %q, want %q", query, want)
	}
}

func TestJQLBuilderValidatesSortWithoutAdditionalFilters(t *testing.T) {
	_, err := (jql.BuildOptions{OrderBy: "updated DESC; DROP"}).Build()
	if err == nil {
		t.Fatal("Build() accepted unsafe order-by field")
	}
}

func TestIssueListJQLCombinesRawJQLWithBuilderFilters(t *testing.T) {
	query, err := jql.IssueList("project = RAW", jql.BuildOptions{Projects: []string{"BUILT"}})
	if err != nil {
		t.Fatalf("IssueList() error = %v", err)
	}
	if query != `project = BUILT AND (project = RAW)` {
		t.Fatalf("IssueList() = %q", query)
	}
}

func TestIssueListJQLLeavesRawJQLAloneWithoutImplicitFilters(t *testing.T) {
	query, err := jql.IssueList("project = RAW ORDER BY created DESC", jql.BuildOptions{})
	if err != nil {
		t.Fatalf("IssueList() error = %v", err)
	}
	if query != "project = RAW ORDER BY created DESC" {
		t.Fatalf("IssueList() = %q", query)
	}
}

func TestIssueMineJQLCombinesRawJQLWithAssignee(t *testing.T) {
	query, err := jql.IssueList("status = Open OR priority = High ORDER BY created DESC", jql.BuildOptions{Assignee: "me"})
	if err != nil {
		t.Fatalf("IssueList() error = %v", err)
	}
	want := `assignee = currentUser() AND (status = Open OR priority = High) ORDER BY created DESC`
	if query != want {
		t.Fatalf("IssueList() = %q, want %q", query, want)
	}
}

// Status comparator filters compile to statusCategory clauses over the
// To Do < In Progress < Done order; ! negates a specific status; plain names
// keep their in-clause.
func TestStatusComparatorFilters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []string
		want     string // substring expected in the built query
		wantErr  bool
	}{
		{name: "plain single", statuses: []string{"Open"}, want: "status = Open"},
		{name: "plain multiple", statuses: []string{"Open", "Closed"}, want: "status in (Open, Closed)"},
		{name: "less than category", statuses: []string{"<Done"}, want: `statusCategory in ("To Do", "In Progress")`},
		{name: "ge category", statuses: []string{">=In Progress"}, want: `statusCategory in ("In Progress", Done)`},
		{name: "gt category single", statuses: []string{">In Progress"}, want: "statusCategory = Done"},
		{name: "le category all", statuses: []string{"<=Done"}, want: `statusCategory in ("To Do", "In Progress", Done)`},
		{name: "negate specific status", statuses: []string{"!Abandoned"}, want: "status != Abandoned"},
		{name: "comparator and negation combine", statuses: []string{">=In Progress", "!Abandoned"}, want: `statusCategory in ("In Progress", Done) AND status != Abandoned`},
		{name: "plain and comparator are OR alternatives", statuses: []string{"Open", ">=In Progress"}, want: `(status = Open OR statusCategory in ("In Progress", Done))`},
		{name: "case-insensitive no-space category alias", statuses: []string{">=inprogress"}, want: `statusCategory in ("In Progress", Done)`},
		{name: "non-category operand errors", statuses: []string{"<Abandoned"}, wantErr: true},
		{name: "empty comparator result errors", statuses: []string{">Done"}, wantErr: true},
		{name: "bare bang errors", statuses: []string{"!"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := jql.BuildOptions{Statuses: tc.statuses}.Build()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Build() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("Build() = %q, want substring %q", got, tc.want)
			}
		})
	}
}

// A custom --jql without its own ORDER BY must still apply --order-by, which
// was previously dropped silently.
func TestIssueListAppliesOrderByToRawWithoutOrderBy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		builder jql.BuildOptions
		want    string
	}{
		{
			name:    "no filters appends order-by",
			raw:     "project = RAW",
			builder: jql.BuildOptions{OrderBy: "updated", Descending: true},
			want:    "project = RAW ORDER BY updated DESC",
		},
		{
			name:    "with filters appends order-by",
			raw:     "project = RAW",
			builder: jql.BuildOptions{Projects: []string{"BUILT"}, OrderBy: "priority"},
			want:    "project = BUILT AND (project = RAW) ORDER BY priority ASC",
		},
		{
			name:    "raw order-by wins over builder",
			raw:     "project = RAW ORDER BY created ASC",
			builder: jql.BuildOptions{OrderBy: "updated", Descending: true},
			want:    "project = RAW ORDER BY created ASC",
		},
		{
			name:    "order-by none appends nothing",
			raw:     "project = RAW",
			builder: jql.BuildOptions{OrderBy: "none"},
			want:    "project = RAW",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, err := jql.IssueList(tc.raw, tc.builder)
			if err != nil {
				t.Fatalf("IssueList() error = %v", err)
			}
			if query != tc.want {
				t.Fatalf("IssueList() = %q, want %q", query, tc.want)
			}
		})
	}
}

// An unsafe --order-by field must be rejected on the custom-JQL path too, not
// only via Build.
func TestIssueListRejectsUnsafeOrderBy(t *testing.T) {
	if _, err := jql.IssueList("project = RAW", jql.BuildOptions{OrderBy: "updated; DROP TABLE x"}); err == nil {
		t.Fatal("IssueList() accepted unsafe order-by field")
	}
}
