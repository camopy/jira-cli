package pipeline_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// In STRICT mode an unresolvable screen schema must ABORT the mutation:
// the CLI cannot tell which custom fields are off-screen, so forwarding
// them would let unvalidated fields reach Jira. The plan makes a
// missing schema fatal in strict mode.
func TestRunMutationStrictAbortsOnUnresolvableSchema(t *testing.T) {
	fetch := func() (pipeline.ScreenSchema, error) {
		return pipeline.ScreenSchema{}, errors.Join(pipeline.ErrSchemaUnknown, errors.New("dial tcp: connection refused"))
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:          adfmode.ModeStrict,
		Fields:        map[string]any{"summary": "ok", "customfield_30001": "x"},
		SchemaFetcher: fetch,
	})
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("strict mode must abort at stage 3 on an unresolvable schema, got %+v", out)
	}
	if out.Err == nil {
		t.Fatal("strict abort must carry an error")
	}
	// The underlying transport cause must survive in the message.
	if got := out.Err.Error(); !strings.Contains(got, "connection refused") {
		t.Fatalf("strict abort error must preserve the underlying cause, got %q", got)
	}
}

// A 404 / unknown-project schema miss is ALWAYS fatal — best-effort
// included. An unknown project is a user error, not a transient outage;
// the known-safe fallback must not paper over it.
func TestRunMutationBestEffortAbortsOnSchemaNotFound(t *testing.T) {
	fetch := func() (pipeline.ScreenSchema, error) {
		return pipeline.ScreenSchema{}, errors.Join(pipeline.ErrSchemaNotFound, errors.New("project NOPE does not exist"))
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:          adfmode.ModeBestEffort,
		Fields:        map[string]any{"summary": "ok"},
		SchemaFetcher: fetch,
	})
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("a not-found schema must abort even in best-effort, got %+v", out)
	}
}

// A transient transport failure in BEST-EFFORT mode still falls back to
// the known-safe field set rather than aborting — the existing
// best-effort contract.
func TestRunMutationBestEffortFallsBackOnTransientSchemaMiss(t *testing.T) {
	fetch := func() (pipeline.ScreenSchema, error) {
		return pipeline.ScreenSchema{}, errors.Join(pipeline.ErrSchemaUnknown, errors.New("timeout"))
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:          adfmode.ModeBestEffort,
		Fields:        map[string]any{"summary": "ok", "customfield_30002": "x"},
		SchemaFetcher: fetch,
	})
	if out.Aborted {
		t.Fatalf("best-effort transient schema miss must not abort, got %+v", out)
	}
	if _, has := out.SubmitFields["customfield_30002"]; has {
		t.Fatalf("a non-known-safe field must be stripped on best-effort fallback, got %v", out.SubmitFields)
	}
}
