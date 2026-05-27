package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

func TestIssueViewPlainRendersReadableIssue(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}

	var buf bytes.Buffer
	err = WriteCommandPlain(&buf, "issue.view", map[string]any{
		"issue": &jira.Issue{
			Key: jira.String("PROJ-1"),
			Fields: &jira.IssueFields{
				Summary:     jira.String("Readable issue"),
				Status:      &jira.Status{Name: jira.String("In Progress")},
				Priority:    &jira.Priority{Name: jira.String("High")},
				Description: &doc,
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	got := buf.String()
	if strings.Contains(got, `issue="{`) || strings.Contains(got, `\"fields\"`) {
		t.Fatalf("issue view rendered escape-encoded JSON:\n%s", got)
	}
	for _, want := range []string{"PROJ-1", "Readable issue", "In Progress", "High", "hello world"} {
		if !strings.Contains(got, want) {
			t.Fatalf("issue view output missing %q:\n%s", want, got)
		}
	}
}

func TestIssueTransitionsPlainRendersReadableTable(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "issue.transitions", map[string]any{
		"issue": "PROJ-1",
		"transitions": []*jira.Transition{
			{ID: jira.String("11"), Name: jira.String("To Do")},
			{ID: jira.String("21"), Name: jira.String("In Progress")},
		},
	})
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	got := buf.String()
	if strings.Contains(got, `transitions="[{`) || strings.Contains(got, `\"id\"`) {
		t.Fatalf("transitions rendered escape-encoded JSON:\n%s", got)
	}
	for _, want := range []string{"Transitions on PROJ-1", "11", "To Do", "21", "In Progress"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transition output missing %q:\n%s", want, got)
		}
	}
}
