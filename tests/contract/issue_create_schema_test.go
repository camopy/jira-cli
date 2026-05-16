package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestIssueCreateJSONSchemaDryRunNoInput(t *testing.T) {
	// Spec: --no-input requires complete input. Supply project_key + issue_type
	// + summary explicitly via --json-input so the headless contract holds.
	path := writeTempJSON(t, `{"project_key":"PROJ","issue_type":"Task","summary":"hello"}`)
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create", "--dry-run", "--no-input", "--json-input", path, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue create output is not JSON: %v\n%s", err, out)
	}
	if env["data"] == nil {
		t.Fatalf("issue create missing data: %+v", env)
	}
}
