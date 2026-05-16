package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestIssueEditDryRunEditNoInputContract(t *testing.T) {
	// --no-input must include at least one field to mutate; --summary is
	// the cheapest field to exercise the dry-run envelope path. An empty
	// edit under --no-input is now a validation error.
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
		"--summary", "renamed", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue edit output is not JSON: %v\n%s", err, out)
	}
	if env["data"] == nil {
		t.Fatalf("issue edit missing data: %+v", env)
	}
}

// The previous TestIssueEditJiraEditorEnvOverridesEditor and
// TestIssueEditBareCommandOpensEditorAndUpdatesADF tests in this file
// exercised the editor flow end-to-end through the CLI binary. The
// agent-mode gate in commands.go now correctly refuses the editor flow
// in non-TTY contexts (which is what `go test` produces), so those
// end-to-end tests have been moved to internal/editor/roundtrip_test.go
// where they exercise EditMarkdown() directly without going through
// the gated cmd layer.
