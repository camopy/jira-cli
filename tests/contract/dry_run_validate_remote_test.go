package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cache"
)

// --validate-remote turns a local dry-run into a server-validated
// pre-flight: read-only metadata fetches (createmeta / editmeta / the
// transitions list) drive the same stage-3/4 validation a live submit
// gets, while the write endpoint is never touched. Without the flag,
// dry-run stays fully local (pinned by dry_run_no_live_calls_test.go).

// createMetaServer serves the two createmeta reads and fails the test on
// any write.
func createMetaServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/PROJ/issuetypes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"issueTypes":[{"id":"10001","name":"Task"}]}`))
	})
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/PROJ/issuetypes/10001", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":2,"fields":[` +
			`{"fieldId":"summary","name":"Summary","required":true,"schema":{"type":"string"}},` +
			`{"fieldId":"labels","name":"Labels","required":false,"schema":{"type":"array","items":"string"}}]}`))
	})
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a --validate-remote dry-run must never POST the create")
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestValidateRemoteRequiresDryRun(t *testing.T) {
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "create", "--no-input", "--validate-remote",
		"--summary", "x", "--project", "PROJ", "--type", "Task",
		"--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("--validate-remote without --dry-run must be refused:\n%s", out)
	}
	if !strings.Contains(string(out), "--dry-run") {
		t.Fatalf("refusal must point at --dry-run: %s", out)
	}
}

func TestCreateValidateRemoteCatchesUnknownField(t *testing.T) {
	srv := createMetaServer(t)
	cfg := jiraConfig(t, srv.URL)
	payload := writeJSON(t, "create.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"x","totally_not_a_field":"y"}}`)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--dry-run", "--validate-remote",
		"--json-input", payload, "--output=json")
	if code != 3 {
		t.Fatalf("unknown field must fail the pre-flight with exit 3, got %d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(string(stdout), "totally_not_a_field") {
		t.Fatalf("error must name the unknown field:\n%s", stdout)
	}
}

func TestCreateValidateRemoteAcceptsCleanPayload(t *testing.T) {
	srv := createMetaServer(t)
	cfg := jiraConfig(t, srv.URL)
	payload := writeJSON(t, "create.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"clean","labels":["ok"]}}`)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--dry-run", "--validate-remote",
		"--json-input", payload, "--output=json")
	if code != 0 {
		t.Fatalf("clean payload must pass the pre-flight, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			ValidatedRemotely bool `json:"validated_remotely"`
			DryRun            bool `json:"dry_run"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(stdout, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %v\n%s", jerr, stdout)
	}
	if !env.Data.ValidatedRemotely || !env.Data.DryRun {
		t.Fatalf("envelope must confirm the remote validation ran on a dry-run: %s", stdout)
	}
}

func TestCreateValidateRemoteRejectsUnknownIssueType(t *testing.T) {
	srv := createMetaServer(t)
	cfg := jiraConfig(t, srv.URL)
	payload := writeJSON(t, "create.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Sprint"},"summary":"x"}}`)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--dry-run", "--validate-remote",
		"--json-input", payload, "--output=json")
	if code == 0 {
		t.Fatalf("an issue type missing from the create screen must fail the pre-flight:\nstdout=%s", stdout)
	}
	combined := string(stdout) + string(stderr)
	if !strings.Contains(combined, "Sprint") {
		t.Fatalf("error must name the unknown issue type: %s", combined)
	}
}

func TestEditValidateRemoteChecksEditScreen(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/editmeta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fields":{"labels":{"name":"Labels","required":false,"schema":{"type":"array","items":"string"}}}}`))
	})
	mux.HandleFunc("PUT /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a --validate-remote dry-run must never PUT the edit")
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := jiraConfig(t, srv.URL)

	payload := writeJSON(t, "edit.json", `{"fields":{"bogus_field":"x"}}`)
	stdout, _, code := runJira(t, "--config", cfg,
		"issue", "edit", "PROJ-1", "--no-input", "--dry-run", "--validate-remote",
		"--json-input", payload, "--output=json")
	if code != 3 {
		t.Fatalf("a field missing from the edit screen must fail the pre-flight with exit 3, got %d\n%s", code, stdout)
	}
	if !strings.Contains(string(stdout), "bogus_field") {
		t.Fatalf("error must name the field:\n%s", stdout)
	}

	good := writeJSON(t, "good.json", `{"fields":{"labels":["fine"]}}`)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "edit", "PROJ-1", "--no-input", "--dry-run", "--validate-remote",
		"--json-input", good, "--output=json")
	if code != 0 {
		t.Fatalf("on-screen field must pass, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(string(stdout), `"validated_remotely":true`) {
		t.Fatalf("envelope must confirm the remote validation ran: %s", stdout)
	}
}

func TestTransitionValidateRemoteResolvesTarget(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"Done","hasScreen":true}]}`))
	})
	mux.HandleFunc("POST /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a --validate-remote dry-run must never POST the transition")
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := jiraConfig(t, srv.URL)

	stdout, _, code := runJira(t, "--config", cfg,
		"issue", "transition", "PROJ-1", "--transition", "99999",
		"--dry-run", "--validate-remote", "--no-input", "--output=json")
	if code != 3 {
		t.Fatalf("a bogus transition id must fail the pre-flight with exit 3, got %d\n%s", code, stdout)
	}
	if !strings.Contains(string(stdout), "no transition matching") {
		t.Fatalf("error must explain the miss:\n%s", stdout)
	}

	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "transition", "PROJ-1", "Done",
		"--dry-run", "--validate-remote", "--no-input", "--output=json")
	if code != 0 {
		t.Fatalf("a valid target must resolve, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	var env struct {
		Data struct {
			Transition          string `json:"transition"`
			TransitionValidated bool   `json:"transition_validated"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(stdout, &env); jerr != nil {
		t.Fatalf("decode: %v\n%s", jerr, stdout)
	}
	if env.Data.Transition != "31" || !env.Data.TransitionValidated {
		t.Fatalf("preview must carry the resolved id and the validated marker: %s", stdout)
	}
}

// A priority name is checked against the cached site priorities whenever
// that cache exists — on a fully local dry-run, before any network call.
func TestPriorityValidatedAgainstCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a local dry-run must not contact Jira: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cfg := jiraConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	key := cacheKeyForTestConfig(t, cfg, "default", srv.URL)
	if _, err := cache.Write(key, "priorities", json.RawMessage(`[{"id":"1","name":"Highest"},{"id":"2","name":"High"},{"id":"3","name":"Medium"}]`)); err != nil {
		t.Fatalf("seed priorities cache: %v", err)
	}

	payload := writeJSON(t, "create.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"x","priority":{"name":"NotARealPriority"}}}`)
	stdout, _, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--dry-run",
		"--json-input", payload, "--output=json")
	if code != 3 {
		t.Fatalf("a priority outside the cached set must exit 3, got %d\n%s", code, stdout)
	}
	if !strings.Contains(string(stdout), "NotARealPriority") || !strings.Contains(string(stdout), "Highest") {
		t.Fatalf("error must name the bad value and the valid set:\n%s", stdout)
	}

	good := writeJSON(t, "good.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"x","priority":{"name":"high"}}}`)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--dry-run",
		"--json-input", good, "--output=json")
	if code != 0 {
		t.Fatalf("a cached priority (case-insensitive) must pass, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
}
