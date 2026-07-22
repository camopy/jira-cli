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
		Name       string           `json:"name"`
		Children   []map[string]any `json:"children"`
		Extensions map[string]any   `json:"extensions"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("root output is not JSON: %v\n%s", err, out)
	}
	if decoded.Name != "jira" || len(decoded.Children) == 0 {
		t.Fatalf("root JSON missing discovery schema: %+v", decoded)
	}
	if decoded.Extensions["contract_version"] == nil {
		t.Fatalf("root discovery schema missing contract_version extension")
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
