package contract

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// When project/issue-type schema is unknown at validation time, the
// CLI MUST attempt EXACTLY ONE schema refresh. If the schema is still
// unknown after that refresh:
//
//	strict mode      — abort with validation error
//	best-effort mode — submit only fields drawn from the known-safe
//	                   set, with a warning per skipped unknown field
//
// The test exercises pipeline.ResolveScreenSchema with a fake fetcher
// that tracks how many times it was called.
func TestSchemaUnknownTriggersExactlyOneRefresh(t *testing.T) {
	var calls atomic.Int64
	fetcher := func() (pipeline.ScreenSchema, error) {
		calls.Add(1)
		return pipeline.ScreenSchema{}, pipeline.ErrSchemaUnknown // both attempts return unknown
	}

	_, _, err := pipeline.ResolveScreenSchemaStrict(fetcher)
	if err == nil {
		t.Fatal("strict mode must abort when schema unknown after refresh")
	}
	if !errors.Is(err, pipeline.ErrSchemaUnknown) {
		t.Fatalf("expected ErrSchemaUnknown, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly 2 calls (initial + 1 refresh), got %d", calls.Load())
	}
}

func TestSchemaUnknownInBestEffortFallsBackToKnownSafe(t *testing.T) {
	var calls atomic.Int64
	fetcher := func() (pipeline.ScreenSchema, error) {
		calls.Add(1)
		return pipeline.ScreenSchema{}, pipeline.ErrSchemaUnknown
	}

	in := map[string]any{
		"summary":           "ok",
		"description":       "ok",
		"customfield_10001": "drop me",
	}
	out, warnings, err := pipeline.ResolveAndApplySchema(in, fetcher, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("best-effort must not abort, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly 2 fetcher calls (initial + 1 refresh), got %d", calls.Load())
	}
	if _, has := out["customfield_10001"]; has {
		t.Errorf("non-known-safe field should be dropped under fallback")
	}
	if out["summary"] != "ok" {
		t.Errorf("known-safe summary should be preserved")
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 drop warning, got %d", len(warnings))
	}
}

// A successful refresh on the first retry MUST NOT trigger a fallback.
// Validation continues against the freshly-loaded schema.
func TestSchemaRecoveredOnRefresh(t *testing.T) {
	var calls atomic.Int64
	fetcher := func() (pipeline.ScreenSchema, error) {
		n := calls.Add(1)
		if n == 1 {
			return pipeline.ScreenSchema{}, pipeline.ErrSchemaUnknown
		}
		return pipeline.ScreenSchema{
			Project:     "KAN",
			IssueType:   "Story",
			ValidFields: map[string]bool{"summary": true},
		}, nil
	}
	schema, _, err := pipeline.ResolveScreenSchemaStrict(fetcher)
	if err != nil {
		t.Fatalf("recovered refresh must succeed, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
	if !schema.ValidFields["summary"] {
		t.Fatalf("recovered schema missing expected field")
	}
}
