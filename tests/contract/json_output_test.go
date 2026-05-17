package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestJSONEnvelopeAndOutputModeConflicts(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--output=json", "agent", "schema")
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

	// The removed legacy boolean flags must be rejected as unknown
	// flags — never silently re-aliased onto a mode.
	for _, removed := range []string{"--json", "--compact", "--plain", "--raw"} {
		c := exec.Command("go", "run", "../../cmd/jira", removed, "agent", "schema")
		if err := c.Run(); err == nil {
			t.Fatalf("removed flag %q was accepted; want unknown-flag error", removed)
		}
	}

	// An invalid --output value is rejected.
	if err := exec.Command("go", "run", "../../cmd/jira", "--output=garbage", "agent", "schema").Run(); err == nil {
		t.Fatal("invalid --output value was accepted")
	}
}

func TestOutputModesApplyToGenericCommands(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--output=compact", "agent", "schema")
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
	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=human", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --plain error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "INF") || !strings.Contains(string(out), `jql="project = PROJ"`) || !strings.Contains(string(out), "searched issues") {
		t.Fatalf("search --plain output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=compact", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --output=compact error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || !strings.Contains(string(out), `"jql":"project = PROJ"`) {
		t.Fatalf("search --output=compact output = %s", out)
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
