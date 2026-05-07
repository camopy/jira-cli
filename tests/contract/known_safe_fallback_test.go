package contract

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/pipeline"
)

// When project/issue-type schema is unknown after one refresh,
// best-effort MUST submit ONLY fields drawn from the known-safe set:
//
//	project, issuetype, summary, description (post ADF validate),
//	labels, priority (by name), assignee (accountId or null),
//	parent (subtask only).
//
// Custom fields, components, versions, fixVersions, epic/sprint
// fields, and any unknown IDs MUST be excluded.
func TestKnownSafeSetIncludesExactlyTheSpecFields(t *testing.T) {
	included := []string{
		"project", "issuetype", "summary", "description",
		"labels", "priority", "assignee", "parent",
	}
	excluded := []string{
		"components", "versions", "fixVersions",
		"epic_link", "sprint_id", "customfield_10001",
		"customfield_99999", "story_points",
	}

	for _, f := range included {
		if !pipeline.IsKnownSafeField(f) {
			t.Errorf("known-safe: %q MUST be in the set", f)
		}
	}
	for _, f := range excluded {
		if pipeline.IsKnownSafeField(f) {
			t.Errorf("known-safe: %q MUST NOT be in the set", f)
		}
	}
}

func TestKnownSafeSetSize(t *testing.T) {
	// Exact whitelist size — adding to the set is a spec amendment.
	expected := 8
	if got := len(pipeline.KnownSafeFields()); got != expected {
		t.Fatalf("known-safe set size = %d, want %d (the set is exact)", got, expected)
	}
}

// ApplyKnownSafeFallback strips every field outside the known-safe
// set and emits a warning per dropped field.
func TestApplyKnownSafeFallbackStripsAndWarns(t *testing.T) {
	in := map[string]any{
		"summary":           "ok",
		"description":       map[string]any{"type": "doc", "version": 1},
		"labels":            []string{"bug"},
		"customfield_10001": "secret",
		"epic_link":         "EPIC-1",
	}
	out, warnings := pipeline.ApplyKnownSafeFallback(in)
	if _, has := out["customfield_10001"]; has {
		t.Errorf("customfield_10001 should be dropped under known-safe fallback")
	}
	if _, has := out["epic_link"]; has {
		t.Errorf("epic_link should be dropped under known-safe fallback")
	}
	if out["summary"] != "ok" {
		t.Errorf("known-safe summary lost or mutated: %v", out["summary"])
	}
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings (cf+epic), got %d", len(warnings))
	}
}
