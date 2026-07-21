// Compact mode has no envelope, so `meta.pagination` cannot ride there — the
// cmdutil compact path folds the pagination block into the data payload
// instead (dataAsMap round-trips the typed SearchJQLOutput so the key merges
// in). That fold was fixed for typed Output structs but untested; this pins
// that `search jql --output=compact` still carries pagination, and carries the
// resume cursor a paged response returns.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestSearchJQLCompactCarriesPagination(t *testing.T) {
	bin := buildJiraBinary(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
			return
		}
		// nextPageToken makes isLast false and gives the fold a resume cursor
		// to carry, so the assertion pins the token, not just the key.
		_, _ = w.Write([]byte(`{"nextPageToken":"CURSOR2","issues":[{"id":"10001","key":"PROJ-1","self":"http://example.invalid/rest/api/3/issue/10001","fields":{"summary":"Checkout returns 500","status":{"name":"Done","statusCategory":{"key":"done","colorName":"green"}},"priority":{"name":"High"},"updated":"2026-05-03T10:00:00Z"}}]}`))
	}))
	t.Cleanup(srv.Close)
	cfg := jiraConfig(t, srv.URL)

	out, err := exec.Command(bin, "--config", cfg, "--output=compact", "search", "jql", "project = PROJ").CombinedOutput()
	if err != nil {
		t.Fatalf("search jql --output=compact error = %v\n%s", err, out)
	}

	// Compact emits the bare data payload — no {ok, meta, data} envelope — so
	// the pagination block lives at the top level of the object.
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("decode compact payload: %v\n%s", err, out)
	}
	if _, ok := payload["issues"]; !ok {
		t.Fatalf("compact payload dropped issues (should be the bare data object):\n%s", out)
	}
	pagination, ok := payload["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("compact search jql did not fold pagination into the data payload:\n%s", out)
	}
	if isLast, _ := pagination["isLast"].(bool); isLast {
		t.Fatalf("pagination.isLast = true, want false for a response with a nextPageToken:\n%s", out)
	}
	if got, _ := pagination["nextCursor"].(string); got != "CURSOR2" {
		t.Fatalf("pagination.nextCursor = %q, want %q:\n%s", got, "CURSOR2", out)
	}
}
