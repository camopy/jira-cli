package contract

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestOutputFailureTaxonomyContract(t *testing.T) {
	env := cli.ErrorEnvelope("issue.edit", cli.NewOutputError(errors.New("write sentinel")))
	if env.Meta.ExitCode == nil || *env.Meta.ExitCode != 8 {
		t.Fatalf("meta.exit_code = %v, want 8", env.Meta.ExitCode)
	}
	if len(env.Errors) != 1 {
		t.Fatalf("errors length = %d, want 1", len(env.Errors))
	}
	got := env.Errors[0]
	if got.Code != "output_write_failed" || got.Type != "io" || got.Retryable {
		t.Fatalf("output error = %+v, want output_write_failed/io/non-retryable", got)
	}
}
