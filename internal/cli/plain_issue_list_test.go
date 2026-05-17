package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/gechr/x/ansi"
	"github.com/matcra587/jira-cli/internal/jira"
)

func TestIssueListPlainTableUsesPrimerFlexLinksAndStyles(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"detail": false,
		"issues": []map[string]any{
			{
				"key":      "SAM1-7",
				"summary":  "Create wallet integration with a long enough summary to exercise flex columns",
				"status":   "In Progress",
				"assignee": "Riley Chen",
				"priority": "High",
			},
		},
	}

	var buf bytes.Buffer
	err := WriteCommandPlain(
		&buf,
		"issue.list",
		data,
		WithPlainBaseURL("https://acme.atlassian.net/"),
		WithPlainTermWidth(72),
		WithPlainTTY(true),
	)
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}

	got := buf.String()
	stripped := ansi.Strip(got)
	for _, want := range []string{"INF", "listed issues", "KEY", "SUMMARY", "STATUS", "ASSIGNEE", "PRIORITY", "SAM1-7", "In Progress", "Riley Chen", "High"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("plain issue list missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "\x1b]8;;https://acme.atlassian.net/browse/SAM1-7") {
		t.Fatalf("issue key is not an OSC 8 Jira hyperlink:\n%q", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("status/priority cells were not styled with ANSI colors:\n%q", got)
	}
	if !regexp.MustCompile("\x1b\\[38;[^m]*mRiley Chen").MatchString(got) {
		t.Fatalf("assignee cell was not color-styled by hash:\n%q", got)
	}

	for _, line := range strings.Split(stripped, "\n") {
		if strings.Contains(line, "SAM1-7") && ansi.StringWidth(line) > 72 {
			t.Fatalf("issue table row exceeded terminal width: width=%d line=%q", ansi.StringWidth(line), line)
		}
	}
}

func TestIssueListPlainDetailRendersFullIssuesAsTable(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"detail": true,
		"issues": []*jira.Issue{
			{
				Key: jira.String("SAM1-7"),
				Fields: &jira.IssueFields{
					Summary:  jira.String("Create wallet integration"),
					Status:   &jira.Status{Name: jira.String("In Progress")},
					Assignee: &jira.User{DisplayName: jira.String("Riley Chen")},
					Priority: &jira.Priority{Name: jira.String("High")},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "issue.list", data, WithPlainTTY(false), WithPlainTermWidth(100))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}

	got := buf.String()
	for _, want := range []string{"listed issues", "KEY", "SUMMARY", "ASSIGNEE", "SAM1-7", "Create wallet integration", "Riley Chen"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail issue list missing %q:\n%s", want, got)
		}
	}
	for _, notWant := range []string{"issues=\"", "\"fields\"", "\"comments\"", "value="} {
		if strings.Contains(got, notWant) {
			t.Fatalf("detail issue list fell back to raw struct output %q:\n%s", notWant, got)
		}
	}
}
