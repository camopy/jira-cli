package contract

// End-to-end: mutating commands under --dry-run must not contact
// Jira at all. Each case points the CLI at a server that fails the test
// on any request, then asserts the dry-run still succeeds locally.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func failOnAnyRequestServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		t.Errorf("dry-run made a live request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestMutationCommandsDryRunMakeNoLiveCalls(t *testing.T) {
	createPayload := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(createPayload, []byte(`{"summary":"Hi","project_key":"PROJ","issue_type":"Task"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"issue.create", []string{"issue", "create", "--dry-run", "--no-input", "--json-input", createPayload}},
		{"issue.edit", []string{"issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--summary", "renamed"}},
		{"issue.transition", []string{"issue", "transition", "PROJ-1", "--dry-run", "--transition", "31"}},
		{"issue.delete", []string{"issue", "delete", "PROJ-1", "--dry-run", "--no-input"}},
		{"issue.comment.add", []string{"issue", "comment", "add", "PROJ-1", "--dry-run", "--markdown", "hello"}},
		{"issue.link", []string{"issue", "link", "PROJ-1", "--dry-run", "--to", "PROJ-2", "--type", "Blocks"}},
		{"issue.link.delete", []string{"issue", "link", "delete", "PROJ-1", "9000", "--dry-run", "--no-input"}},
		{"issue.weblink", []string{"issue", "weblink", "PROJ-1", "--dry-run", "--url", "https://example.com"}},
		{"worklog.add", []string{"worklog", "add", "PROJ-1", "--dry-run", "--time-spent", "1h"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, hits := failOnAnyRequestServer(t)
			cfg := jiraConfig(t, srv.URL)
			args := append([]string{"--config", cfg, "--output=json"}, c.args...)
			stdout, stderr, code := runJira(t, args...)
			if code != 0 {
				t.Fatalf("%s --dry-run exit = %d\nstdout=%s\nstderr=%s", c.name, code, stdout, stderr)
			}
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Fatalf("%s --dry-run made %d live request(s); dry-run must be local-only", c.name, n)
			}
		})
	}
}
