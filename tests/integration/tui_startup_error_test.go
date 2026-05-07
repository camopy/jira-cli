package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/tui"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestTUIRendersStartupLoadErrorsInline(t *testing.T) {
	app := tui.NewWithOptions(t.Context(), tui.Options{
		IssueProvider: tui.IssueProviderFunc(func(context.Context) ([]*jira.Issue, error) {
			return nil, errors.New("credential for profile \"work\" is required: credential not found")
		}),
	})

	rendered := app.View().Content
	for _, want := range []string{"credential for profile \"work\" is required", "No issues"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("TUI startup error view missing %q:\n%s", want, rendered)
		}
	}
}
