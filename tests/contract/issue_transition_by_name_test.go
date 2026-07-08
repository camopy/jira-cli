package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// transitionNameServer serves a fixed transition list on GET and records the
// id POSTed back, so the end-to-end name→id resolution path can be asserted.
func transitionNameServer(t *testing.T, postedID *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-1/transitions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"transitions":[{"id":"21","name":"In Progress"},{"id":"41","name":"Done"}]}`)
		case http.MethodPost:
			var body struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode transition body: %v", err)
			}
			*postedID = body.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A human status name passed positionally must be resolved to its transition
// id against the issue's available transitions, then POSTed.
func TestIssueTransitionResolvesStatusNameToID(t *testing.T) {
	var postedID string
	srv := transitionNameServer(t, &postedID)
	cfg := jiraConfig(t, srv.URL)

	out, err := exec.Command(buildJiraBinary(t), "--config", cfg,
		"issue", "transition", "PROJ-1", "In Progress", "--output=json").CombinedOutput()
	if err != nil {
		t.Fatalf("issue transition error = %v\n%s", err, out)
	}
	if postedID != "21" {
		t.Fatalf("server received transition id %q, want 21", postedID)
	}
	var env struct {
		Data struct {
			Transition string `json:"transition"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Transition != "21" {
		t.Fatalf("envelope transition = %q, want resolved id 21", env.Data.Transition)
	}
}

// An unmatched status name exits 3 and lists the available transitions.
func TestIssueTransitionUnknownStatusNameErrorsWithAvailable(t *testing.T) {
	var postedID string
	srv := transitionNameServer(t, &postedID)
	cfg := jiraConfig(t, srv.URL)

	out, err := exec.Command(buildJiraBinary(t), "--config", cfg,
		"issue", "transition", "PROJ-1", "Nope", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown status:\n%s", out)
	}
	if postedID != "" {
		t.Fatalf("no transition should be POSTed for an unknown status; got %q", postedID)
	}
	combined := string(out)
	for _, want := range []string{"no transition matching", "In Progress", "Done"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("unknown-status error missing %q:\n%s", want, combined)
		}
	}
}
