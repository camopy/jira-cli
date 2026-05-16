package contract

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// `jira agent adf-matrix --json` MUST emit the registry as the
// envelope `data` so an agent can consume the support matrix without
// parsing prose. Each row uses the shared envelope shape: `kind`,
// `name`, `status`, `capabilities`, `input_shape`, `output_shape`,
// `warnings`, `official_url`, `notes`, plus `submit_description`.
func TestAgentADFMatrixJSON(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "agent", "adf-matrix", "--output=json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("adf-matrix --json: %v\nstderr: %s", err, exitStderr(err))
	}

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not JSON envelope: %v\n%s", err, out)
	}
	rows, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("envelope.data not an array, got %T", env["data"])
	}
	if len(rows) < 30 {
		t.Fatalf("expected at least 30 rows (MVP set), got %d", len(rows))
	}

	required := []string{"kind", "name", "status", "capabilities", "official_url", "submit_description"}
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("row %d not an object: %T", i, raw)
		}
		for _, key := range required {
			if _, has := row[key]; !has {
				t.Fatalf("row %d (%v) missing required key %q. row: %v", i, row["name"], key, row)
			}
		}
		caps, ok := row["capabilities"].(map[string]any)
		if !ok {
			t.Fatalf("row %d capabilities not object: %v", i, row["capabilities"])
		}
		for _, capKey := range []string{"author", "render", "preserve", "validate", "submit"} {
			if _, has := caps[capKey]; !has {
				t.Fatalf("row %d capabilities missing %q", i, capKey)
			}
		}
	}
}

func exitStderr(err error) string {
	exit := &exec.ExitError{}
	if errors.As(err, &exit) {
		return strings.TrimSpace(string(exit.Stderr))
	}
	return ""
}
