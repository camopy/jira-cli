package contract

import (
	"os/exec"
	"testing"
)

func TestDestructiveIssueCommandsRequireForceOrDryRun(t *testing.T) {
	for _, sub := range []string{"clone", "move", "delete"} {
		cmd := exec.Command("go", "run", "../../cmd/jira", "issue", sub, "PROJ-1")
		if err := cmd.Run(); err == nil {
			t.Fatalf("issue %s without --force/--dry-run succeeded", sub)
		}
		cmd = exec.Command("go", "run", "../../cmd/jira", "issue", sub, "PROJ-1", "--dry-run", "--no-input")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("issue %s dry-run error = %v\n%s", sub, err, out)
		}
	}
}
