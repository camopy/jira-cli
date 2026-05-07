package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIssueCreateDryRunOmitsIssueAndPopulatesPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(`{"project_key":"PROJ","issue_type":"Task","summary":"Hello world"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "create",
		"--dry-run", "--no-input", "--json-input", path, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue create output is not JSON: %v\n%s", err, out)
	}
	if _, has := env.Data["issue"]; has {
		t.Fatalf("dry-run must omit 'issue' (spec: dry_run omits issue, populates preview): %+v", env.Data)
	}
	preview, ok := env.Data["preview"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run missing 'preview' object: %+v", env.Data)
	}
	if preview["summary"] != "Hello world" {
		t.Fatalf("preview.summary = %#v, want \"Hello world\"", preview["summary"])
	}
	if env.Data["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", env.Data["dry_run"])
	}
}

func TestIssueCreateDryRunConvertsMarkdownDescriptionToADF(t *testing.T) {
	payload := `{"summary":"Hi","project_key":"PROJ","issue_type":"Task","description_markdown":"# Heading\n\nBody paragraph"}`
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira", "issue", "create",
		"--dry-run", "--no-input", "--json-input", path, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Preview map[string]any `json:"preview"`
			DryRun  bool           `json:"dry_run"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue create output is not JSON: %v\n%s", err, out)
	}
	description, ok := env.Data.Preview["description_adf"].(map[string]any)
	if !ok {
		t.Fatalf("preview missing description_adf object after Markdown→ADF: %+v", env.Data.Preview)
	}
	if description["type"] != "doc" {
		t.Fatalf("description_adf is not an ADF document: %+v", description)
	}
	if env.Data.Preview["project_key"] != "PROJ" {
		t.Fatalf("preview missing project_key: %+v", env.Data.Preview)
	}
	if env.Data.Preview["issue_type"] != "Task" {
		t.Fatalf("preview missing issue_type: %+v", env.Data.Preview)
	}
}
