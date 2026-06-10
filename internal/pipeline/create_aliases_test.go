package pipeline_test

import (
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
