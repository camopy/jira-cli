package pipeline_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// NormalizeCreateAliases translates the CLI create-input aliases
// (project_key / issue_type / assignee_account_id) into the Jira wire
// field ids (project / issuetype / assignee) BEFORE screen validation.
// Screen schemas key on the wire ids, so an un-normalized alias would
// be rejected as off-screen even for a default create.
func TestNormalizeCreateAliasesTranslatesAliasKeys(t *testing.T) {
	in := map[string]any{
		"project_key":         "JCT",
		"issue_type":          "Task",
		"assignee_account_id": "acc-1",
		"summary":             "ok",
	}
	out := pipeline.NormalizeCreateAliases(in)
	if _, has := out["project_key"]; has {
		t.Fatalf("project_key alias must be removed: %v", out)
	}
	if _, has := out["issue_type"]; has {
		t.Fatalf("issue_type alias must be removed: %v", out)
	}
	if _, has := out["assignee_account_id"]; has {
		t.Fatalf("assignee_account_id alias must be removed: %v", out)
	}
	project, ok := out["project"].(map[string]any)
	if !ok || project["key"] != "JCT" {
		t.Fatalf("project wire shape wrong: %v", out["project"])
	}
	issuetype, ok := out["issuetype"].(map[string]any)
	if !ok || issuetype["name"] != "Task" {
		t.Fatalf("issuetype wire shape wrong: %v", out["issuetype"])
	}
	assignee, ok := out["assignee"].(map[string]any)
	if !ok || assignee["accountId"] != "acc-1" {
		t.Fatalf("assignee wire shape wrong: %v", out["assignee"])
	}
	if out["summary"] != "ok" {
		t.Fatalf("native field mutated by normalization: %v", out["summary"])
	}
}

// An alias whose wire key is also explicitly set is a conflict the CLI
// will not silently resolve — it returns nothing usable, surfacing the
// double-set so the user supplies the field exactly once.
func TestNormalizeCreateAliasesConflictKeepsBothDistinct(t *testing.T) {
	in := map[string]any{
		"project_key": "JCT",
		"project":     map[string]any{"key": "OTHER"},
		"summary":     "ok",
	}
	_, err := pipeline.NormalizeCreateAliasesChecked(in)
	if err == nil {
		t.Fatal("a project_key alias colliding with an explicit project must be a conflict error")
	}
}

// Agreement is not a conflict: an alias and its wire object carrying the
// same identity (however it got there — profile default, duplicated
// input) normalizes cleanly, keeping the explicit wire object.
func TestNormalizeCreateAliasesAgreementIsNotAConflict(t *testing.T) {
	in := map[string]any{
		"project_key": "jct", // case differs; identity matches
		"project":     map[string]any{"key": "JCT"},
		"summary":     "ok",
	}
	out, err := pipeline.NormalizeCreateAliasesChecked(in)
	if err != nil {
		t.Fatalf("matching alias and wire values must not conflict: %v", err)
	}
	project, ok := out["project"].(map[string]any)
	if !ok || project["key"] != "JCT" {
		t.Fatalf("explicit wire object must be kept: %v", out["project"])
	}
	if _, has := out["project_key"]; has {
		t.Fatalf("alias must be removed after agreement: %v", out)
	}
}

// The mismatch error names both values so the caller can see which one
// to drop, rather than a bare "set in two places".
func TestNormalizeCreateAliasesMismatchNamesBothValues(t *testing.T) {
	_, err := pipeline.NormalizeCreateAliasesChecked(map[string]any{
		"project_key": "KAN9",
		"project":     map[string]any{"key": "JCT"},
	})
	if err == nil {
		t.Fatal("mismatched values must conflict")
	}
	for _, want := range []string{"KAN9", "JCT", "project_key", "project"} {
		if !containsString(err.Error(), want) {
			t.Fatalf("conflict error must name %q: %v", want, err)
		}
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && strings.Contains(haystack, needle)
}

// CreateWireValue reads a field's identity from either spelling, which
// requiredness checks and profile-default injection rely on so a
// wire-only payload is never "missing" its own field.
func TestCreateWireValueAcceptsEitherSpelling(t *testing.T) {
	tests := map[string]struct {
		fields map[string]any
		alias  string
		want   string
	}{
		"flat alias":       {map[string]any{"project_key": "JCT"}, "project_key", "JCT"},
		"wire object key":  {map[string]any{"project": map[string]any{"key": "JCT"}}, "project_key", "JCT"},
		"wire object id":   {map[string]any{"project": map[string]any{"id": "10001"}}, "project_key", "10001"},
		"wire bare string": {map[string]any{"project": "JCT"}, "project_key", "JCT"},
		"issuetype name":   {map[string]any{"issuetype": map[string]any{"name": "Task"}}, "issue_type", "Task"},
		"absent":           {map[string]any{}, "project_key", ""},
		"unknown alias":    {map[string]any{"x": "y"}, "not_an_alias", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := pipeline.CreateWireValue(tc.fields, tc.alias); got != tc.want {
				t.Fatalf("CreateWireValue(%v, %q) = %q, want %q", tc.fields, tc.alias, got, tc.want)
			}
		})
	}
}

// A default-project create payload that uses the create aliases must
// survive screen validation: after normalization the wire ids are on
// the create screen, so a resolved schema accepts it in strict mode.
func TestRunMutationAcceptsNormalizedDefaultCreatePayload(t *testing.T) {
	normalized := pipeline.NormalizeCreateAliases(map[string]any{
		"project_key":         "JCT",
		"issue_type":          "Task",
		"assignee_account_id": "acc-1",
		"summary":             "ok",
	})
	schema := pipeline.ScreenSchema{
		Project:   "JCT",
		IssueType: "Task",
		ValidFields: map[string]bool{
			"project": true, "issuetype": true, "assignee": true, "summary": true,
		},
	}
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: normalized,
		Schema: schema,
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("normalized default create payload must pass screen validation, got %+v", out)
	}
}
