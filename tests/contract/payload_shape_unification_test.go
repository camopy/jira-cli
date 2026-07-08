package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// One payload contract across create and edit: the Jira-native
// {"fields": {...}} object and the flat convenience keys are accepted
// interchangeably on BOTH commands, converging to the same result.
// Historically create demanded flat and edit demanded nested — the exact
// opposite shapes for two closely related mutations.

func createPreviewFields(t *testing.T, cfg, payload string) map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := exec.Command(buildJiraBinary(t), "--config", cfg,
		"issue", "create", "--no-input", "--dry-run",
		"--json-input", path, "--output=json").Output()
	if err != nil {
		t.Fatalf("create dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Preview map[string]any `json:"preview"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok create preview: %v\n%s", jerr, out)
	}
	return env.Data.Preview
}

func TestIssueCreateAcceptsBothPayloadShapes(t *testing.T) {
	cfg := jiraConfigNoServer(t)
	flat := `{"summary":"same","project_key":"PROJ","issue_type":"Task"}`
	nested := `{"fields":{"summary":"same","project":{"key":"PROJ"},"issuetype":{"name":"Task"}}}`

	flatPreview := createPreviewFields(t, cfg, flat)
	nestedPreview := createPreviewFields(t, cfg, nested)

	flatJSON, _ := json.Marshal(flatPreview)
	nestedJSON, _ := json.Marshal(nestedPreview)
	if string(flatJSON) != string(nestedJSON) {
		t.Fatalf("both shapes must converge to the same preview:\nflat:   %s\nnested: %s", flatJSON, nestedJSON)
	}
}

func editPreviewFields(t *testing.T, payload string) map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edit.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "edit", "PROJ-1", "--no-input", "--dry-run",
		"--json-input", path, "--output=json").Output()
	if err != nil {
		t.Fatalf("edit dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Fields map[string]any `json:"fields"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok edit preview: %v\n%s", jerr, out)
	}
	return env.Data.Fields
}

func TestIssueEditAcceptsBothPayloadShapes(t *testing.T) {
	nested := `{"fields":{"summary":"same edit"}}`
	flat := `{"summary":"same edit"}`

	nestedFields := editPreviewFields(t, nested)
	flatFields := editPreviewFields(t, flat)

	nestedJSON, _ := json.Marshal(nestedFields)
	flatJSON, _ := json.Marshal(flatFields)
	if string(nestedJSON) != string(flatJSON) {
		t.Fatalf("both shapes must converge to the same fields:\nnested: %s\nflat:   %s", nestedJSON, flatJSON)
	}
	if flatFields["summary"] != "same edit" {
		t.Fatalf("flat edit payload must carry its field through: %v", flatFields)
	}
}

// jiraConfigNoServer writes a profile with defaults but no live base URL
// requirement for dry-run-only flows.
func jiraConfigNoServer(t *testing.T) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"
secret_backend = "keyring"
default_project = "PROJ"
default_issue_type = "Task"
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return cfg
}
