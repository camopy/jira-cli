package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
)

func TestPlainOutputExtractsADFText(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	var buf bytes.Buffer
	if err := cli.WritePlain(&buf, map[string]any{"description": doc}); err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "INF") || !strings.Contains(got, `description="hello world"`) {
		t.Fatalf("plain output = %q", buf.String())
	}
}

// The single-issue human view surfaces the expanded workflow transitions on
// one line so the valid moves are visible without a second command.
func TestIssueViewPlainRendersTransitions(t *testing.T) {
	data := map[string]any{"issue": map[string]any{
		"key":    "PROJ-1",
		"fields": map[string]any{"summary": "hello"},
		"transitions": []any{
			map[string]any{"id": "21", "name": "In Progress"},
			map[string]any{"id": "31", "name": "Done"},
		},
	}}
	var buf bytes.Buffer
	if err := cli.WriteIssueViewPlain(&buf, "issue.view", data); err != nil {
		t.Fatalf("WriteIssueViewPlain() error = %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "transitions: In Progress (21), Done (31)") {
		t.Fatalf("plain issue view missing transitions line:\n%s", got)
	}
}
