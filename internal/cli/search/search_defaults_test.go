package search

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestSearchOutputFieldsDoNotReadMutableIssueListFields(t *testing.T) {
	original := append([]string(nil), jira.IssueListFields...)
	jira.IssueListFields = []string{"evil"}
	defer func() { jira.IssueListFields = original }()

	fields, detailed, err := searchOutputFields(searchOptions{})
	if err != nil {
		t.Fatalf("searchOutputFields() error = %v", err)
	}
	if detailed {
		t.Fatal("searchOutputFields() detailed = true, want false")
	}
	if strings.Join(fields, ",") != strings.Join(jira.DefaultIssueListFields(), ",") {
		t.Fatalf("fields = %v, want default fields %v", fields, jira.DefaultIssueListFields())
	}
}

// --fields narrows the summary projection; only --full switches to the raw
// wire records. A field selector flipping the detailed bit is how the output
// contract silently broke, so pin it.
func TestSearchOutputFieldsSelectorKeepsSummaryProjection(t *testing.T) {
	fields, detailed, err := searchOutputFields(searchOptions{fields: []string{"summary", "status"}})
	if err != nil {
		t.Fatalf("searchOutputFields() error = %v", err)
	}
	if detailed {
		t.Fatal("searchOutputFields() detailed = true with --fields, want false")
	}
	if strings.Join(fields, ",") != "summary,status" {
		t.Fatalf("fields = %v, want [summary status]", fields)
	}
}

func TestSearchOutputFieldsFullIsDetailed(t *testing.T) {
	fields, detailed, err := searchOutputFields(searchOptions{full: true})
	if err != nil {
		t.Fatalf("searchOutputFields() error = %v", err)
	}
	if !detailed {
		t.Fatal("searchOutputFields() detailed = false with --full, want true")
	}
	if strings.Join(fields, ",") != "*all" {
		t.Fatalf("fields = %v, want [*all]", fields)
	}
}
