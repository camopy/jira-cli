package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestIssueListJSONSchemaAndCompact(t *testing.T) {
	cfg := emptyBaseURLConfig(t)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "list", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --json error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue list output is not JSON: %v\n%s", err, out)
	}
	if env["meta"] == nil || env["data"] == nil || env["errors"] == nil {
		t.Fatalf("issue list envelope missing fields: %+v", env)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "list", "--output=compact")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --compact error = %v\n%s", err, out)
	}
	var compact map[string]any
	if err := json.Unmarshal(out, &compact); err != nil {
		t.Fatalf("compact output is not JSON: %v\n%s", err, out)
	}
	if compact["issues"] == nil {
		t.Fatalf("compact output missing issues: %+v", compact)
	}
}
