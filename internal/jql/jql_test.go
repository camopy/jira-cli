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
