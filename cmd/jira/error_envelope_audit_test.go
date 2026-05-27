package main

import (
	"encoding/json"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/boardscope"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
)

// TestBoardValidationStdoutEnvelopeReportsValidationExitCode pins finding A:
// a missing default_board failure carries the literal "not found" substring,
// so the central mapper must still classify it as a validation error (exit 3)
// — never let the substring classifier infer not_found/exit 2.
func TestBoardValidationStdoutEnvelopeReportsValidationExitCode(t *testing.T) {
	cmd, stdout, stderr := outputModeTestCommand(cli.ModeJSON)
	bve := boardscope.ValidationError{
		Msg: jira.DefaultBoardMissingMessage("default", "Engineering Sprint"),
	}
	if err := writeErrorEnvelopeToStderr(cmd, bve); err != nil {
		t.Fatalf("writeErrorEnvelopeToStderr() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty success channel on error", stdout.String())
	}

	var env struct {
		Meta struct {
			ExitCode *int `json:"exit_code"`
		} `json:"meta"`
		Errors []struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	if env.Meta.ExitCode == nil || *env.Meta.ExitCode != 3 {
		t.Fatalf("meta.exit_code = %v, want 3", env.Meta.ExitCode)
	}
	if len(env.Errors) != 1 {
		t.Fatalf("errors[] = %v, want exactly 1", env.Errors)
	}
	if env.Errors[0].Type != "validation" {
		t.Errorf("errors[0].type = %q, want validation", env.Errors[0].Type)
	}
	if env.Errors[0].Code == "" {
		t.Errorf("errors[0].code is empty, want a stable validation code")
	}
}

// TestFoldRawWarningsIntoDataPreservesNonMapPayload pins finding B for the raw
// warning slice: a non-map compact payload must still carry warnings rather
// than silently dropping them.
func TestFoldRawWarningsIntoDataPreservesNonMapPayload(t *testing.T) {
	data := []any{"a", "b"}
	warnings := []map[string]any{{"type": "credential_cleanup", "message": "stale keyring entry"}}

	out := cmdutil.FoldRawWarningsIntoData(data, warnings)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("cmdutil.FoldRawWarningsIntoData() dropped non-map payload, got %T", out)
	}
	if m["data"] == nil {
		t.Errorf("wrapped payload missing data key: %v", m)
	}
	if w, _ := m["warnings"].([]map[string]any); len(w) != 1 {
		t.Errorf("wrapped payload missing warnings: %v", m)
	}
}

// TestFoldWarningsIntoDataPreservesNonMapPayload pins finding B for the typed
// cli.Warning slice.
func TestFoldWarningsIntoDataPreservesNonMapPayload(t *testing.T) {
	data := "scalar-payload"
	warnings := []cli.Warning{{Type: "credential_cleanup", Message: "stale keyring entry"}}

	out := cmdutil.FoldWarningsIntoData(data, warnings)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("cmdutil.FoldWarningsIntoData() dropped non-map payload, got %T", out)
	}
	if m["data"] != "scalar-payload" {
		t.Errorf("wrapped payload data = %v, want scalar-payload", m["data"])
	}
	if w, _ := m["warnings"].([]cli.Warning); len(w) != 1 {
		t.Errorf("wrapped payload missing warnings: %v", m)
	}
}
