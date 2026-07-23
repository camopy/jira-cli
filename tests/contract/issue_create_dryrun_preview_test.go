package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueCreateDryRunOmitsIssueAndPopulatesPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(`{"project_key":"PROJ","issue_type":"Task","summary":"Hello world"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t), "issue", "create",
		"--dry-run", "--no-input", "--json-input", path, "--output=json")
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
	if validated, exists := env.Data["validated_remotely"]; !exists || validated != false {
		t.Fatalf("validated_remotely = %#v (present=%v), want explicit false", validated, exists)
	}
}

func TestIssueCreateConvenienceFlagsPopulatePreview(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "issue", "create",
		"--summary", "Example issue summary",
		"--project", "PROJ",
		"--type", "Task",
		"--parent", "PROJ-1",
		"--priority", "High",
		"--label", "alpha",
		"--label", "beta",
		"--dry-run", "--no-input", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Preview map[string]any `json:"preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue create output is not JSON: %v\n%s", err, out)
	}
	p := env.Data.Preview
	if p["project_key"] != "PROJ" || p["issue_type"] != "Task" {
		t.Fatalf("preview missing project/type from flags: %+v", p)
	}
	if p["summary"] != "Example issue summary" {
		t.Fatalf("preview.summary = %#v", p["summary"])
	}
	if parent, ok := p["parent"].(map[string]any); !ok || parent["key"] != "PROJ-1" {
		t.Fatalf("preview.parent = %#v, want {key: PROJ-1}", p["parent"])
	}
	if priority, ok := p["priority"].(map[string]any); !ok || priority["name"] != "High" {
		t.Fatalf("preview.priority = %#v, want {name: High}", p["priority"])
	}
	labels, ok := p["labels"].([]any)
	if !ok || len(labels) != 2 || labels[0] != "alpha" || labels[1] != "beta" {
		t.Fatalf("preview.labels = %#v, want [alpha beta]", p["labels"])
	}
}

func TestIssueCreateProjectFlagConflictsWithWireProjectInJSON(t *testing.T) {
	// --project (the project_key alias) plus a raw wire "project" in
	// --json-input must error rather than silently pick a winner.
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(`{"summary":"S","issue_type":"Task","project":{"key":"FROMJSON"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t), "issue", "create",
		"--project", "FROMFLAG", "--json-input", path,
		"--dry-run", "--no-input", "--output=json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected conflict error, got success:\n%s", out)
	}
	if !strings.Contains(string(out), "different values") {
		t.Fatalf("expected alias-vs-wire mismatch error naming both values, got:\n%s", out)
	}
}

func TestIssueCreateDryRunConvertsMarkdownDescriptionToADF(t *testing.T) {
	payload := `{"summary":"Hi","project_key":"PROJ","issue_type":"Task","description_markdown":"# Heading\n\nBody paragraph"}`
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t), "issue", "create",
		"--dry-run", "--no-input", "--json-input", path, "--output=json")
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
