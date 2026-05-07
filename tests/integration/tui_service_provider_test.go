package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/tui"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestTUIUsesIssueProviderInsteadOfHardcodedIssue(t *testing.T) {
	app := tui.NewWithOptions(t.Context(), tui.Options{
		IssueProvider: tui.IssueProviderFunc(func(context.Context) ([]*jira.Issue, error) {
			return []*jira.Issue{{
				Key: jira.String("SRV-7"),
				Fields: &jira.IssueFields{
					Summary: jira.String("Loaded from service"),
					Status:  &jira.Status{Name: jira.String("Done")},
				},
			}}, nil
		}),
	})
	rendered := app.View().Content
	for _, want := range []string{"SRV-7", "Loaded from service", "Done"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("provider-backed TUI missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "PROJ-1") || strings.Contains(rendered, "Investigate login timeout") {
		t.Fatalf("provider-backed TUI still rendered hardcoded issue:\n%s", rendered)
	}
}
