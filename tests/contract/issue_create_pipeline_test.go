package contract

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// Every mutation command MUST route through pipeline.RunMutation. This
// contract exercises the higher-level entry point with realistic inputs
// (a fields map, an ADF doc, a schema fetcher, a customfield registry).
//
// The lower-level pipeline.Run stays as the per-stage fatal-semantics
// test seam; RunMutation is what internal/cli/root/commands.go calls in
// production.
func TestRunMutationStrictBlocksInvalidField(t *testing.T) {
	in := pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"summary":           "ok",
			"description":       map[string]any{"type": "doc", "version": 1},
			"customfield_99999": "should fail",
		},
		Schema: pipeline.ScreenSchema{
			Project:     "JCT",
			IssueType:   "Story",
			ValidFields: map[string]bool{"summary": true, "description": true},
		},
	}
	out := pipeline.RunMutation(in)
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("expected stage-3 abort, got %+v", out)
	}
	var fve *pipeline.FieldValidationError
	if !errors.As(out.Err, &fve) {
		t.Fatalf("expected FieldValidationError, got %T: %v", out.Err, out.Err)
	}
	if fve.Field != "customfield_99999" {
		t.Fatalf("wrong field reported: %q", fve.Field)
	}
}

func TestRunMutationBestEffortDropsAndContinues(t *testing.T) {
	in := pipeline.MutationInput{
		Mode: adfmode.ModeBestEffort,
		Fields: map[string]any{
			"summary":           "ok",
			"customfield_99999": "drop me",
		},
		Schema: pipeline.ScreenSchema{
			Project:     "JCT",
			IssueType:   "Story",
			ValidFields: map[string]bool{"summary": true},
		},
		DryRun: true, // don't actually try to submit in test
	}
	out := pipeline.RunMutation(in)
	if out.Aborted {
		t.Fatalf("best-effort drop must not abort: %+v", out)
	}
	if !out.PreviewReady {
		t.Fatal("dry-run should report PreviewReady")
	}
	if _, has := out.SubmitFields["customfield_99999"]; has {
		t.Fatalf("invalid field not dropped: %v", out.SubmitFields)
	}
	if len(out.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(out.Warnings))
	}
}

// : unknown customfield IDs forwarded without schema MUST surface
// in the warnings array. The spec (attack 3 pass criterion) says:
// "accepted and forwarded opaquely" is valid, BUT "behavior must be
// consistent and surface in the warnings array if forwarded".
// Without a schema (stage 3 skipped), stage 4 must emit one warning per
// customfield_XXXXXX key that is not in the type registry.
func TestRunMutationNoSchemaUnknownCustomfieldWarns(t *testing.T) {
	in := pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"summary":              "ok",
			"customfield_999999":   "foo",
			"customfield_10010":    float64(3),
			"known_native_summary": "x", // non-customfield passes silently
		},
		// No Schema, no SchemaFetcher — stage 3 is skipped.
		DryRun: true,
	}
	out := pipeline.RunMutation(in)
	if out.Aborted {
		t.Fatalf("unknown customfield forwarding must not abort: %+v", out)
	}
	// Both customfield_XXXXXX keys must appear in warnings.
	warnedFields := map[string]bool{}
	for _, w := range out.Warnings {
		if w.Type == "customfield_unknown_type" {
			warnedFields[w.Field] = true
		}
	}
	for _, cf := range []string{"customfield_999999", "customfield_10010"} {
		if !warnedFields[cf] {
			t.Errorf("expected warning for %q (type=customfield_unknown_type), got warnings: %v", cf, out.Warnings)
		}
	}
	// known_native_summary is not a customfield pattern — must NOT warn.
	if warnedFields["known_native_summary"] {
		t.Errorf("non-customfield key must not generate customfield_unknown_type warning")
	}
	// Both fields must still be forwarded (opaque pass-through).
	if _, has := out.SubmitFields["customfield_999999"]; !has {
		t.Errorf("customfield_999999 must be forwarded (opaque), got SubmitFields: %v", out.SubmitFields)
	}
	if _, has := out.SubmitFields["customfield_10010"]; !has {
		t.Errorf("customfield_10010 must be forwarded (opaque), got SubmitFields: %v", out.SubmitFields)
	}
}

// Schema-resolver integration: when the schema fetcher returns
// ErrSchemaUnknown, RunMutation MUST attempt one refresh, then fall
// back to the known-safe set (best-effort) or abort (strict).
func TestRunMutationFallsBackToKnownSafeWhenSchemaUnknown(t *testing.T) {
	calls := 0
	fetcher := func() (pipeline.ScreenSchema, error) {
		calls++
		return pipeline.ScreenSchema{}, pipeline.ErrSchemaUnknown
	}
	in := pipeline.MutationInput{
		Mode: adfmode.ModeBestEffort,
		Fields: map[string]any{
			"summary":           "ok",
			"customfield_99999": "drop me",
		},
		SchemaFetcher: fetcher,
		DryRun:        true,
	}
	out := pipeline.RunMutation(in)
	if out.Aborted {
		t.Fatalf("best-effort fallback must not abort, got %+v", out)
	}
	if calls != 2 {
		t.Fatalf("expected 2 fetcher calls (initial + 1 refresh), got %d", calls)
	}
	if _, has := out.SubmitFields["customfield_99999"]; has {
		t.Fatalf("non-known-safe field not dropped: %v", out.SubmitFields)
	}
	if out.SubmitFields["summary"] != "ok" {
		t.Fatalf("known-safe summary missing: %v", out.SubmitFields)
	}
	if len(out.Warnings) == 0 {
		t.Fatalf("expected drop warning")
	}
}
