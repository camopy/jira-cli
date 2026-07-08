package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempJSON(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestIssueCreateNoInputDryRunRejectsMissingRequired enforces the spec rule:
// "Headless write commands require complete JSON input via `--no-input` +
// `--json-input`; without these, must refuse with validation error rather than
// synthesize success." Even with `--dry-run`, missing required fields must
// fail at the input boundary, not silently flow through with empty values.
func TestIssueCreateNoInputDryRunRejectsMissingRequiredFields(t *testing.T) {
	cfg := emptyBaseURLConfig(t)
	cmd := exec.Command(buildJiraBinary(t),
		"--config", cfg, "issue", "create", "--no-input", "--dry-run", "--output=json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("issue create --no-input --dry-run with no required fields succeeded:\n%s", out)
	}
	got := strings.ToLower(string(out))
	if !strings.Contains(got, "summary") {
		t.Fatalf("expected validation error mentioning summary, got: %s", out)
	}
	if !strings.Contains(got, "required") && !strings.Contains(got, "missing") {
		t.Fatalf("expected validation/missing-fields error, got: %s", out)
	}
}

// TestIssueCreateNoInputDryRunAcceptsCompleteJSONInput is the positive case:
// when --no-input is paired with --json-input that supplies project_key,
// issue_type, and summary, the dry-run preview succeeds.
func TestIssueCreateNoInputDryRunAcceptsCompleteJSONInput(t *testing.T) {
	path := writeTempJSON(t, `{"project_key":"PROJ","issue_type":"Task","summary":"Hello"}`)
	cfg := emptyBaseURLConfig(t)
	cmd := exec.Command(buildJiraBinary(t), "--config", cfg,
		"issue", "create", "--no-input", "--dry-run", "--json-input", path, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create with complete json-input failed: %v\n%s", err, out)
	}
}
