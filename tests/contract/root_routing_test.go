package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestRootNonTTYJSONDiscovery(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("root command error = %v\n%s", err, out)
	}
	var decoded struct {
		Meta map[string]any `json:"meta"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("root output is not JSON: %v\n%s", err, out)
	}
	if decoded.Meta == nil || decoded.Data["commands"] == nil {
		t.Fatalf("root JSON missing envelope commands: %+v", decoded)
	}
}

func TestRootInteractiveFlagRequiresTTY(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "-i")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("jira -i succeeded without a TTY:\n%s", out)
	}
	if !strings.Contains(string(out), "tui requires an interactive terminal") {
		t.Fatalf("jira -i non-tty error = %s", out)
	}
}
