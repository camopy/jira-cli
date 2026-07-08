package contract

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestReadOnlyModeBlocksMutationsAtHTTPLayer enforces the design rule that
// JIRA_READ_ONLY refusals come from the Jira client (single source of truth)
// rather than per-command boilerplate. Adding a new mutation command must
// not require adding a separate read-only check; this test catches that
// regression by exercising several distinct mutation paths and confirming
// they all refuse identically.
func TestReadOnlyModeBlocksMutationsAtHTTPLayer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read-only mode allowed a Jira API request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	editPayload := writeIssueEditPayload(t)
	createPayload := writeIssueCreatePayload(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"issue create", []string{"issue", "create", "--no-input", "--json-input", createPayload, "--output=json"}},
		{"issue edit", []string{"issue", "edit", "PROJ-1", "--no-input", "--json-input", editPayload, "--output=json"}},
		{"issue comment", []string{"issue", "comment", "PROJ-1", "--markdown", "x", "--no-input", "--output=json"}},
		{"worklog add", []string{"worklog", "add", "PROJ-1", "--time-spent", "30m", "--no-input", "--output=json"}},
		{"epic add", []string{"epic", "add", "PROJ-1", "EPIC-1", "--no-input", "--output=json"}},
	} {
		args := append([]string{"--config", cfg}, tc.args...)
		cmd := exec.Command(buildJiraBinary(t), args...)
		cmd.Env = append(os.Environ(), "JIRA_READ_ONLY=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("%s succeeded with JIRA_READ_ONLY=1:\n%s", tc.name, out)
		}
		got := string(out)
		if !strings.Contains(strings.ToLower(got), "read-only") {
			t.Fatalf("%s did not mention read-only in error:\n%s", tc.name, out)
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
			t.Fatalf("%s did not exit with validation code 3: err=%v\n%s", tc.name, err, out)
		}
	}
}

// TestReadOnlyModeAllowsReadsAndDryRuns confirms the gate is targeted at
// state-changing methods only — reads (GET) and --dry-run paths (no API
// call) must continue to work so an agent can safely explore.
func TestReadOnlyModeAllowsReadsAndDryRuns(t *testing.T) {
	cfg := jiraConfig(t, "")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"auth status local check", []string{"--output=json", "auth", "status", "--no-probe"}},
		{"issue create dry-run", []string{
			"issue", "create", "--dry-run", "--no-input",
			"--json-input", writeIssueCreatePayload(t), "--output=json",
		}},
	} {
		args := append([]string{"--config", cfg}, tc.args...)
		cmd := exec.Command(buildJiraBinary(t), args...)
		cmd.Env = append(os.Environ(), "JIRA_READ_ONLY=1", "JIRA_TOKEN_DEFAULT=test-token")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed under read-only mode:\n%s\nerr=%v", tc.name, out, err)
		}
	}
}
