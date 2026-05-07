package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// `jira agent fieldtypes --json` MUST emit the customfield registry as
// the envelope `data` using the shared envelope shape so consumers
// parse both ADF + customfield surfaces with the same code.
func TestAgentFieldTypesJSON(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "agent", "fieldtypes", "--json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fieldtypes --json: %v\nstderr: %s", err, exitStderr(err))
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	rows, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data not array: %T", env["data"])
	}
	if len(rows) < 14 {
		t.Fatalf("expected at least 14 rows (minimum field types), got %d", len(rows))
	}
	required := []string{"kind", "name", "status", "capabilities", "submit_description"}
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("row %d not an object", i)
		}
		for _, key := range required {
			if _, has := row[key]; !has {
				t.Fatalf("row %d (%v) missing %q", i, row["name"], key)
			}
		}
	}
}
