package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestJSONEnvelopeAndOutputModeConflicts(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "--output=json", "agent", "schema")
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
		c := exec.Command(buildJiraBinary(t), removed, "agent", "schema")
		if err := c.Run(); err == nil {
			t.Fatalf("removed flag %q was accepted; want unknown-flag error", removed)
		}
	}

	// An invalid --output value is rejected.
	if err := exec.Command(buildJiraBinary(t), "--output=garbage", "agent", "schema").Run(); err == nil {
		t.Fatal("invalid --output value was accepted")
	}
}

func TestOutputModesApplyToGenericCommands(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "--output=compact", "agent", "schema")
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
	cmd = exec.Command(buildJiraBinary(t), "--config", cfg, "--output=human", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --plain error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "INF") || !strings.Contains(string(out), `jql="project = PROJ"`) || !strings.Contains(string(out), "Searched issues") {
		t.Fatalf("search --plain output = %s", out)
	}

	cmd = exec.Command(buildJiraBinary(t), "--config", cfg, "--output=compact", "search", "jql", "project = PROJ")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search --output=compact error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || !strings.Contains(string(out), `"jql":"project = PROJ"`) {
		t.Fatalf("search --output=compact output = %s", out)
	}
}

func TestAgentStructuredCommandsRenderJSONInHumanMode(t *testing.T) {
	bin := buildJiraBinary(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "schema", args: []string{"agent", "schema"}},
		{name: "adf-matrix", args: []string{"agent", "adf-matrix"}},
		{name: "fieldtypes", args: []string{"agent", "fieldtypes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--output=human"}, tc.args...)
			out, err := exec.Command(bin, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("jira %v error = %v\n%s", args, err, out)
			}
			if strings.Contains(string(out), "INF") {
				t.Fatalf("agent command used clog key-value output instead of JSON:\n%s", out)
			}
			if !strings.Contains(string(out), "\n  \"") {
				t.Fatalf("agent command human JSON was not pretty-printed:\n%s", out)
			}
			var env map[string]any
			if err := json.Unmarshal(out, &env); err != nil {
				t.Fatalf("agent command human output is not JSON: %v\n%s", err, out)
			}
			if env["meta"] == nil || env["data"] == nil || env["errors"] == nil {
				t.Fatalf("agent command human JSON is not an envelope: %+v", env)
			}
		})
	}
}

func TestAgentDetectionDefaultsToCompactJSON(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "--config", emptyBaseURLConfig(t), "search", "jql", "project = PROJ")
	cmd.Env = append(cmd.Environ(), "CLAUDE_CODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent compact command error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), `"meta"`) || !strings.Contains(string(out), `"jql":"project = PROJ"`) {
		t.Fatalf("agent compact output = %s", out)
	}
}
