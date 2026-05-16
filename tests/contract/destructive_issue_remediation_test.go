package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestForcedCloneAndMoveAreHTTPBackedOrExplicitUnsupported(t *testing.T) {
	bin := buildJiraBinary(t)
	for _, tc := range []struct {
		sub        string
		wantMethod string
		wantPath   string
	}{
		{sub: "clone", wantMethod: http.MethodPost, wantPath: "/rest/api/3/issue"},
		{sub: "move", wantMethod: http.MethodPut, wantPath: "/rest/api/3/issue/PROJ-1"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			var called bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Clone requires a GET of the source issue before the mutating
				// request.  Serve a minimal fixture so the command can proceed.
				if tc.sub == "clone" && r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1" {
					_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Original","issuetype":{"name":"Task"},"project":{"key":"PROJ"}}}`))
					return
				}
				called = true
				if r.Method != tc.wantMethod || r.URL.Path != tc.wantPath {
					t.Fatalf("unexpected request for issue %s: %s %s", tc.sub, r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				fields, ok := body["fields"].(map[string]any)
				if !ok || fields["summary"] != "Moved or cloned" {
					t.Fatalf("issue %s request body missing JSON input fields: %+v", tc.sub, body)
				}
				_, _ = w.Write([]byte(`{"key":"PROJ-2"}`))
			}))
			defer srv.Close()

			input := filepath.Join(t.TempDir(), "issue.json")
			if err := os.WriteFile(input, []byte(`{"fields":{"summary":"Moved or cloned"}}`), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			cfg := jiraConfig(t, srv.URL)
			cmd := exec.Command(bin, "--config", cfg, "issue", tc.sub, "PROJ-1", "--force", "--no-input", "--output=json", "--json-input", input)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("issue %s should be HTTP-backed, got error = %v\n%s", tc.sub, err, out)
			}
			if !called {
				t.Fatalf("issue %s reported success without making an HTTP-backed Jira mutation:\n%s", tc.sub, out)
			}
		})
	}
}
