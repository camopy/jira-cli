package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// worklog add --json-input must accept the canonical ADF `comment`
// field — a raw ADF document — the same shape comment/create accept,
// not only the `comment_markdown` string form.
func TestADFInputShape_WorklogAcceptsCanonicalComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worklog-comment-adf.json")
	body := `{"time_spent":"30m","comment":{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"text","text":"paired"}]}
	]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"--output=json", "worklog", "add", "PROJ-1",
		"--json-input", path, "--dry-run", "--no-input")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add error = %v\n%s", err, out)
	}
	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output not JSON: %v\n%s", err, out)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("canonical ADF comment must be accepted; got %+v\n%s", env.Errors, out)
	}
}

// worklog add --json-input with an invalid nested ADF comment doc must
// fail before submission — the canonical comment field is validated
// just like comment/create ADF input.
func TestADFInputShape_WorklogRejectsInvalidComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worklog-bad-comment-adf.json")
	body := `{"time_spent":"30m","comment":{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x"},
			{"type":"unknown_magic_node"}
		]}
	]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"--output=json", "worklog", "add", "PROJ-1",
		"--json-input", path, "--dry-run", "--no-input")
	out, runErr := cmd.Output()
	if runErr == nil {
		t.Fatalf("invalid ADF comment must fail; command succeeded\n%s", out)
	}
	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output not JSON: %v\n%s", err, out)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("invalid ADF comment must be rejected; got zero errors\n%s", out)
	}
}

// agent schema must publish the canonical ADF input shape so dry-run,
// live submit, and the schema surface describe the same fields.
func TestADFInputShape_AgentSchemaDescribesCanonicalShape(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--output=json", "agent", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			InputSchemas map[string]any `json:"input_schemas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("agent schema output not JSON: %v\n%s", err, out)
	}
	if env.Data.InputSchemas == nil {
		t.Fatalf("agent schema must publish input_schemas describing the canonical ADF shape\n%s", out)
	}
	if _, ok := env.Data.InputSchemas["adf_document"]; !ok {
		t.Fatalf("input_schemas must include the canonical adf_document shape; got keys %v", keysOfAny(env.Data.InputSchemas))
	}
}

func keysOfAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
