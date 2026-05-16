package contract

// Regression coverage: every mutating command must submit the
// validated, post-pipeline payload — never the raw pre-validation input.
//
// The pipeline runs in best-effort mode here so that an invalid
// registered customfield is *dropped* (with a warning) rather than
// aborting. If a command submits the original input map, the dropped
// field reaches the wire; these tests capture the live request body and
// fail when that happens.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// captureServer records the body of the first mutating request it sees.
type captureServer struct {
	mu   sync.Mutex
	body map[string]any
	path string
}

func (c *captureServer) record(r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = r.URL.Path
	c.body = map[string]any{}
	_ = json.Unmarshal(raw, &c.body)
}

func (c *captureServer) capturedBody() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

// TestIssueCreateSubmitsValidatedFields proves issue create sends the
// pipeline's SubmitFields. A best-effort-invalid `number` customfield is
// dropped by stage 4; the live POST body must not contain it.
func TestIssueCreateSubmitsValidatedFields(t *testing.T) {
	cap := &captureServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","key":"PROJ-1","self":"x"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := `{"summary":"Hi","project_key":"PROJ","issue_type":"Task","number":"not-a-number"}`
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg, "--adf-best-effort",
		"issue", "create", "--no-input", "--json-input", path, "--output=json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	body := cap.capturedBody()
	if body == nil {
		t.Fatalf("no POST body captured")
	}
	fields, _ := body["fields"].(map[string]any)
	if fields == nil {
		// Some shapes nest under the top-level map directly.
		fields = body
	}
	if _, leaked := fields["number"]; leaked {
		t.Fatalf("invalid customfield 'number' reached the wire — submission used pre-validation payload, not SubmitFields: %#v", body)
	}
}

// TestIssueEditSubmitsValidatedFields proves issue edit sends the
// pipeline's SubmitFields on the live PUT.
func TestIssueEditSubmitsValidatedFields(t *testing.T) {
	cap := &captureServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := `{"fields":{"summary":"renamed","number":"not-a-number"}}`
	path := filepath.Join(t.TempDir(), "edit.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg, "--adf-best-effort",
		"issue", "edit", "PROJ-1", "--no-input", "--json-input", path, "--output=json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	body := cap.capturedBody()
	if body == nil {
		t.Fatalf("no PUT body captured")
	}
	fields, _ := body["fields"].(map[string]any)
	if fields == nil {
		t.Fatalf("PUT body missing fields object: %#v", body)
	}
	if _, leaked := fields["number"]; leaked {
		t.Fatalf("invalid customfield 'number' reached the wire on issue edit — submission used pre-validation fields: %#v", fields)
	}
}

// TestIssueCloneSubmitsValidatedFields proves clone sends SubmitFields.
func TestIssueCloneSubmitsValidatedFields(t *testing.T) {
	cap := &captureServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","key":"PROJ-1","fields":{"summary":"orig","project":{"key":"PROJ"},"issuetype":{"name":"Task"}}}`)
	})
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"2","key":"PROJ-2","self":"x"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := `{"fields":{"number":"not-a-number"}}`
	path := filepath.Join(t.TempDir(), "clone.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg, "--adf-best-effort",
		"issue", "clone", "PROJ-1", "--force", "--json-input", path, "--output=json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	body := cap.capturedBody()
	if body == nil {
		t.Fatalf("no clone POST body captured")
	}
	fields, _ := body["fields"].(map[string]any)
	if fields == nil {
		fields = body
	}
	if _, leaked := fields["number"]; leaked {
		t.Fatalf("invalid customfield 'number' reached the wire on issue clone: %#v", body)
	}
}

// TestIssueCreateDryRunPreviewMatchesSubmitFields proves the dry-run
// preview renders the validated SubmitFields — a field dropped by the
// pipeline must not appear in the preview the operator inspects.
func TestIssueCreateDryRunPreviewMatchesSubmitFields(t *testing.T) {
	payload := `{"summary":"Hi","project_key":"PROJ","issue_type":"Task","number":"not-a-number"}`
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, stderr, code := runJira(t, "--adf-best-effort",
		"issue", "create", "--dry-run", "--no-input", "--json-input", path, "--output=json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var env struct {
		Data struct {
			Preview map[string]any `json:"preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, stdout)
	}
	if _, leaked := env.Data.Preview["number"]; leaked {
		t.Fatalf("dry-run preview shows a field the pipeline dropped — preview does not reflect SubmitFields: %#v", env.Data.Preview)
	}
}
