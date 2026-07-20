// Input-validation gaps in the key/flag layer: the single-issue attachment
// verbs must route their key argument through the shared issue-key parser
// (a traversal path or hallucinated key fails fast, exit 3, instead of an
// ok dry-run), and negative numeric flags must be rejected the way a
// non-numeric value already is — never silently degraded into the default.
// Zero stays each flag's documented disabled/default sentinel.
package contract

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func runExpectingValidationExit(t *testing.T, bin, wantInError string, args ...string) {
	t.Helper()
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err == nil {
		t.Fatalf("%v succeeded, want validation failure:\n%s", args, out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("%v exit = %v, want validation exit 3\n%s", args, err, out)
	}
	if !strings.Contains(string(out), wantInError) {
		t.Fatalf("%v error should mention %q; got:\n%s", args, wantInError, out)
	}
}

func TestAttachmentDownloadValidatesIssueKey(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	// A traversal path or hallucinated key must fail fast — the dry-run
	// used to return ok/exit 0 for it.
	runExpectingValidationExit(t, bin, "invalid issue key",
		"--config", cfg, "--output=json", "issue", "attachment", "download", "../PROJ-1", "10500", "--dry-run")
	runExpectingValidationExit(t, bin, "invalid issue key",
		"--config", cfg, "--output=json", "issue", "attachment", "download", "not-a-key", "10500", "--dry-run")
	// download addresses exactly one issue; a range expansion is rejected.
	runExpectingValidationExit(t, bin, "single issue key",
		"--config", cfg, "--output=json", "issue", "attachment", "download", "PROJ-1:3", "10500", "--dry-run")

	// A canonical key keeps working, with the validated key echoed back.
	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"issue", "attachment", "download", "PROJ-1", "10500", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("valid-key dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Issue struct {
				Key string `json:"key"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if env.Data.Issue.Key != "PROJ-1" {
		t.Fatalf("dry-run issue key = %q, want PROJ-1\n%s", env.Data.Issue.Key, out)
	}
}

func TestAttachmentDeleteValidatesIssueKey(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	runExpectingValidationExit(t, bin, "invalid issue key",
		"--config", cfg, "--output=json", "issue", "attachment", "delete", "../PROJ-1", "10500", "--dry-run")
}

func TestNegativeNumericFlagsAreRejected(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	// Persistent duration flags, validated before any command logic runs.
	runExpectingValidationExit(t, bin, "--timeout",
		"--config", cfg, "--output=json", "version", "--timeout", "-10s")
	runExpectingValidationExit(t, bin, "--max-retry-wait",
		"--config", cfg, "--output=json", "version", "--max-retry-wait", "-5s")
	// Per-command page size.
	runExpectingValidationExit(t, bin, "--limit",
		"--config", cfg, "--output=json", "search", "jql", "project = X", "--limit", "-5")
}

// --limit 0 is the documented default sentinel: the request Jira sees still
// carries the default page size, and the command exits 0.
func TestLimitZeroKeepsDefaultPageSize(t *testing.T) {
	bin := buildJiraBinary(t)
	var gotMaxResults int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxResults int `json:"maxResults"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode search body: %v", err)
		}
		gotMaxResults = body.MaxResults
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[]}`))
	}))
	defer srv.Close()
	cfg := jiraConfig(t, srv.URL)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"search", "jql", "project = X", "--limit", "0").CombinedOutput()
	if err != nil {
		t.Fatalf("search jql --limit 0 error = %v\n%s", err, out)
	}
	if gotMaxResults != 50 {
		t.Fatalf("search jql --limit 0 requested maxResults = %d, want the default 50", gotMaxResults)
	}
}
