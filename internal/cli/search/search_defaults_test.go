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
