package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// issue create --description-markdown that drops content during ADF
// conversion (a GFM table has no authoring path) must abort in the
// default strict mode rather than silently submitting a document with
// the table missing.
func TestMarkdownStrictAbort_CreateRejectsLossyTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "create-lossy-md.json")
	body := "{\"summary\":\"x\",\"project_key\":\"JCT\",\"issue_type\":\"Task\"," +
		"\"description_markdown\":\"intro\\n\\n| a | b |\\n|---|---|\\n| 1 | 2 |\\n\"}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json")

	var env struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, stderr, _ := runCommandExpectErrorEnvelope(t, cmd, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("strict mode must abort on lossy Markdown conversion; got zero errors\n%s", stderr)
	}
}

// In best-effort mode the same lossy Markdown succeeds (dry-run) with a
// warning naming the dropped construct.
func TestMarkdownStrictAbort_CreateBestEffortWarns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "create-lossy-md-be.json")
	body := "{\"summary\":\"x\",\"project_key\":\"JCT\",\"issue_type\":\"Task\"," +
		"\"description_markdown\":\"intro\\n\\n| a | b |\\n|---|---|\\n| 1 | 2 |\\n\"}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json", "--adf-best-effort")
	stdout, _ := cmd.Output()

	var env struct {
		Errors   []map[string]any `json:"errors"`
		Warnings []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, stdout)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("best-effort must not error on lossy Markdown; got %+v\n%s", env.Errors, stdout)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("best-effort must warn about lossy Markdown; got zero warnings\n%s", stdout)
	}
}
