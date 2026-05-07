package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestJSONEnvelopeAndOutputModeConflicts(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--json", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	if env["meta"] == nil || env["errors"] == nil {
		t.Fatalf("schema envelope missing fields: %+v", env)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--compact", "--plain", "schema")
	if err := cmd.Run(); err == nil {
		t.Fatal("combined output modes succeeded")
	}
}

func TestOutputModesApplyToGenericCommands(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--compact", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema --compact error = %v\n%s", err, out)
	}
	var compact map[string]any
	if err := json.Unmarshal(out, &compact); err != nil {
		t.Fatalf("schema --compact output is not JSON: %v\n%s", err, out)
	}
	if compact["meta"] != nil || compact["commands"] == nil {
		t.Fatalf("schema --compact output = %+v", compact)
	}

	cfg := emptyBaseURLConfig(t)
	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--plain", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --plain error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "INF") || !strings.Contains(string(out), `jql="project = PROJ"`) || !strings.Contains(string(out), "searched issues") {
		t.Fatalf("search --plain output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--raw", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --raw error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || !strings.Contains(string(out), `"jql":"project = PROJ"`) {
		t.Fatalf("search --raw output = %s", out)
	}
}

func TestAgentDetectionDefaultsToCompactJSON(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", emptyBaseURLConfig(t), "search", "jql", "project = PROJ")
	cmd.Env = append(cmd.Environ(), "CLAUDE_CODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent compact command error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || !strings.Contains(string(out), `"jql":"project = PROJ"`) {
		t.Fatalf("agent compact output = %s", out)
	}
}
