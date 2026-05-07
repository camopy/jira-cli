package contract

// T101a: parametric error-contract coverage for every new command in
// The error contract is:
//
//   401/403 → exit 1, errors[].type = "auth"
//   404     → exit 2, errors[].type = "not_found"
//   429     → exit 4, errors[].type = "rate_limit"
//   500     → exit 5, errors[].type = "server"
//
// Each command is driven against a fake server that always returns the
// configured status. Pre-flight resolves (e.g. /myself, /user/search)
// are answered with the same status so the failure path fires on the
// first wire call and never gets short-circuited by a downstream parse
// error in our test harness.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errorContractCases drives every new command once against a server that
// answers the configured status for every request. Pre-flight calls
// (/myself etc.) are also covered so the very first wire failure
// dominates regardless of which path the command takes first.
type errorContractCase struct {
	name string
	args []string // appended to the {--config, cfg, --json} prefix
}

func errorContractStatusServer(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"errorMessages":["upstream"],"errors":{}}`))
	}))
}

// statusCases maps HTTP status to expected (exit code, errors[].type).
var statusCases = []struct {
	status   int
	exitCode int
	errType  string
}{
	{http.StatusUnauthorized, 1, "auth"},
	{http.StatusNotFound, 2, "not_found"},
	{http.StatusTooManyRequests, 4, "rate_limit"},
	{http.StatusInternalServerError, 5, "server"},
}

// TestLifecycleCommandsErrorContract is the parametric (command × status)
// matrix asserted in T101a.
func TestLifecycleCommandsErrorContract(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(tmpFile, []byte("a"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bodyJSON := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(bodyJSON, []byte(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []errorContractCase{
		{"attachment.list", []string{"issue", "attachment", "list", "PROJ-1"}},
		{"attachment.add", []string{"issue", "attachment", "add", "PROJ-1", "--file", tmpFile}},
		{"attachment.delete", []string{"issue", "attachment", "delete", "PROJ-1", "99", "--force"}},
		{"attachment.download", []string{"issue", "attachment", "download", "PROJ-1", "99", "--output", filepath.Join(t.TempDir(), "out.bin")}},
		{"comment.list", []string{"issue", "comment", "list", "PROJ-1"}},
		{"comment.add", []string{"issue", "comment", "add", "PROJ-1", "--json-input", bodyJSON, "--no-input"}},
		{"comment.edit", []string{"issue", "comment", "edit", "PROJ-1", "500", "--json-input", bodyJSON, "--no-input"}},
		{"comment.delete", []string{"issue", "comment", "delete", "PROJ-1", "500", "--force"}},
		{"watchers.list", []string{"issue", "watchers", "list", "PROJ-1"}},
		{"watchers.add.me", []string{"issue", "watchers", "add", "PROJ-1", "--user", "me"}},
		{"watchers.remove.me", []string{"issue", "watchers", "remove", "PROJ-1", "--user", "me"}},
		{"watch", []string{"issue", "watch", "PROJ-1"}},
		{"unwatch", []string{"issue", "unwatch", "PROJ-1"}},
		{"link.list", []string{"issue", "link", "list", "PROJ-1"}},
		{"link.delete", []string{"issue", "link", "delete", "PROJ-1", "10000", "--force"}},
		{"link.types", []string{"issue", "link", "types"}},
		{"cache.linktypes", []string{"cache", "linktypes"}},
		// 003: boards primer + listing.
		{"cache.boards", []string{"cache", "boards"}},
		{"boards.list", []string{"boards", "list"}},
	}

	for _, sc := range statusCases {
		for _, c := range cases {
			name := c.name + "/" + http.StatusText(sc.status)
			t.Run(name, func(t *testing.T) {
				srv := errorContractStatusServer(sc.status)
				defer srv.Close()
				cfg := jiraConfig(t, srv.URL)
				t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
				// Pin the cache to a per-test tempdir so commands that
				// short-circuit through the local cache (link types,
				// cache linktypes) still go through the wire path and
				// observe the configured failure status.
				t.Setenv("XDG_CACHE_HOME", t.TempDir())

				args := append([]string{"--config", cfg, "--json"}, c.args...)
				stdout, stderr, code := runJira(t, args...)
				if code != sc.exitCode {
					t.Fatalf("exit = %d; want %d (status %d)\nstdout=%s\nstderr=%s",
						code, sc.exitCode, sc.status, stdout, stderr)
				}
				// Envelope is mandatory for every error path under --json.
				var env map[string]any
				if err := json.Unmarshal(stdout, &env); err != nil {
					t.Fatalf("envelope not JSON (status %d): %v\nstdout=%s\nstderr=%s",
						sc.status, err, stdout, stderr)
				}
				errs, _ := env["errors"].([]any)
				if len(errs) == 0 {
					t.Fatalf("errors[] empty (status %d): %s", sc.status, stdout)
				}
				first, _ := errs[0].(map[string]any)
				gotType, _ := first["type"].(string)
				if !strings.EqualFold(gotType, sc.errType) {
					t.Errorf("errors[0].type = %q; want %q (status %d)", gotType, sc.errType, sc.status)
				}
			})
		}
	}
}
