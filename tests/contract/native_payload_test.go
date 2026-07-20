package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Raw Jira REST bodies are first-class mutation input: an agent can paste
// the exact POST /rest/api/3/issue, POST .../transitions, or POST
// /rest/api/3/issueLink body into --json-input and the CLI submits it
// without alias translation, screen-validation mangling, or silent
// section drops.

func writeJSON(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// A native create body keeps its identity wire fields even when the
// tenant's createmeta does not list project / issuetype on the create
// screen — they are structural, and screen validation must never drop
// or reject them.
func TestCreateNativeBodySurvivesScreenValidation(t *testing.T) {
	var posted []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/PROJ/issuetypes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"issueTypes":[{"id":"10001","name":"Task"}]}`))
	})
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/PROJ/issuetypes/10001", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":2,"fields":[` +
			`{"fieldId":"summary","name":"Summary","required":true,"schema":{"type":"string"}},` +
			`{"fieldId":"priority","name":"Priority","required":false,"schema":{"type":"priority"}}]}`))
	})
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"PROJ-9"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := writeJSON(t, "create.json",
		`{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"native body","priority":{"name":"High"}}}`)
	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--json-input", payload, "--output=json")
	if code != 0 {
		t.Fatalf("native create body must submit, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	body := string(posted)
	for _, want := range []string{`"project":{"key":"PROJ"}`, `"issuetype":{"name":"Task"}`, `"priority":{"name":"High"}`, `"summary":"native body"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("create wire body missing %q:\n%s", want, body)
		}
	}
}

// The native transition section names the target inside the payload, so a
// pasted REST body needs no positional STATUS at all.
func TestTransitionNativeBodyDryRun(t *testing.T) {
	payload := writeJSON(t, "transition.json",
		`{"transition":{"id":"31"},"fields":{"resolution":{"name":"Done"}},"update":{"labels":[{"add":"native"}]}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "transition", "PROJ-1", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("native transition dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Transition string         `json:"transition"`
			Fields     map[string]any `json:"fields"`
			Update     map[string]any `json:"update"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %v\n%s", jerr, out)
	}
	if env.Data.Transition != "31" {
		t.Fatalf("target must come from the payload's transition section: %s", out)
	}
	if _, ok := env.Data.Fields["resolution"]; !ok {
		t.Fatalf("preview must carry the fields: %s", out)
	}
	if _, ok := env.Data.Update["labels"]; !ok {
		t.Fatalf("preview must carry the update block: %s", out)
	}
}

func TestTransitionNativeBodySubmitsUpdateBlock(t *testing.T) {
	var posted []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"Done","hasScreen":true}]}`))
	})
	mux.HandleFunc("POST /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := writeJSON(t, "transition.json",
		`{"transition":{"id":"31"},"update":{"labels":[{"add":"native"}]}}`)
	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "transition", "PROJ-1", "--no-input",
		"--json-input", payload, "--output=json")
	if code != 0 {
		t.Fatalf("native transition body must submit, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	body := string(posted)
	for _, want := range []string{`"transition":{"id":"31"}`, `"update"`, `"labels":[{"add":"native"}]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("transition wire body missing %q:\n%s", want, body)
		}
	}
}

// The target may be named in two places; different values are a conflict,
// agreement is not.
func TestTransitionTargetConflictRejected(t *testing.T) {
	payload := writeJSON(t, "transition.json", `{"transition":{"id":"31"}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "transition", "PROJ-1", "Done", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("conflicting targets must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "set twice") {
		t.Fatalf("error must explain the double target: %s", out)
	}
}

func TestTransitionTargetAgreementAccepted(t *testing.T) {
	payload := writeJSON(t, "transition.json", `{"transition":{"name":"done"}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "transition", "PROJ-1", "Done", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("agreeing targets must not conflict: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"ok":true`) && !strings.Contains(string(out), `"ok": true`) {
		t.Fatalf("expected ok envelope:\n%s", out)
	}
}

func TestTransitionUpdateCommentConflictsWithCommentKey(t *testing.T) {
	payload := writeJSON(t, "transition.json",
		`{"transition":{"id":"31"},"comment":{"type":"doc","version":1,"content":[]},"update":{"comment":[{"add":{"body":{"type":"doc","version":1,"content":[]}}}]}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "transition", "PROJ-1", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("update.comment plus a comment input must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "set twice") {
		t.Fatalf("error must explain the double comment: %s", out)
	}
}

// A native edit body's update block is a sibling of fields, never folded
// into the field set, and an update-only edit does not fall through to
// the interactive editor.
func TestEditNativeUpdateBlockDryRun(t *testing.T) {
	payload := writeJSON(t, "edit.json",
		`{"fields":{"labels":["kept"]},"update":{"labels":[{"add":"native"}]}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("edit with update block dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Fields map[string]any `json:"fields"`
			Update map[string]any `json:"update"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %v\n%s", jerr, out)
	}
	if _, ok := env.Data.Update["labels"]; !ok {
		t.Fatalf("preview must carry the update block: %s", out)
	}
	if _, ok := env.Data.Fields["update"]; ok {
		t.Fatalf("the update block must never be folded into fields: %s", out)
	}
}

func TestEditUpdateOnlyPayloadSkipsEditor(t *testing.T) {
	payload := writeJSON(t, "edit.json", `{"update":{"labels":[{"add":"native"}]}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("update-only edit must not demand an editor: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"update"`) {
		t.Fatalf("preview must carry the update block:\n%s", out)
	}
}

func TestEditNativeUpdateBlockSubmits(t *testing.T) {
	var put []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/editmeta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fields":{"labels":{"name":"Labels","required":false,"schema":{"type":"array","items":"string"}}}}`))
	})
	mux.HandleFunc("PUT /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, r *http.Request) {
		put, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := writeJSON(t, "edit.json",
		`{"fields":{"labels":["kept"]},"update":{"labels":[{"add":"native"}]}}`)
	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "edit", "PROJ-1", "--no-input", "--json-input", payload, "--output=json")
	if code != 0 {
		t.Fatalf("edit with update block must submit, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	body := string(put)
	for _, want := range []string{`"fields":{"labels":["kept"]}`, `"update":{"labels":[{"add":"native"}]}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit wire body missing %q:\n%s", want, body)
		}
	}
}

// `issue link --json-input` accepts the exact POST /rest/api/3/issueLink
// body, including the comment block no flag can express.
func TestLinkJSONInputDryRun(t *testing.T) {
	payload := writeJSON(t, "link.json",
		`{"type":{"name":"Blocks"},"inwardIssue":{"key":"PROJ-1"},"outwardIssue":{"key":"PROJ-2"},"comment":{"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"linked"}]}]}}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "link", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("link --json-input dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Inward struct {
				Key string `json:"key"`
			} `json:"inward_issue"`
			Outward struct {
				Key string `json:"key"`
			} `json:"outward_issue"`
			Type    string         `json:"type"`
			Comment map[string]any `json:"comment"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %v\n%s", jerr, out)
	}
	if env.Data.Inward.Key != "PROJ-1" || env.Data.Outward.Key != "PROJ-2" || env.Data.Type != "Blocks" {
		t.Fatalf("preview must carry the body's endpoints and type: %s", out)
	}
	if env.Data.Comment == nil {
		t.Fatalf("preview must carry the comment block: %s", out)
	}
}

func TestLinkJSONInputSubmitsNativeBody(t *testing.T) {
	var posted []byte
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/issueLink", func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := writeJSON(t, "link.json",
		`{"type":{"name":"Blocks"},"inwardIssue":{"key":"PROJ-1"},"outwardIssue":{"key":"PROJ-2"},"comment":{"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"linked"}]}]}}}`)
	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "link", "--no-input", "--json-input", payload, "--output=json")
	if code != 0 {
		t.Fatalf("link --json-input must submit, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	body := string(posted)
	for _, want := range []string{`"type":{"name":"Blocks"}`, `"inwardIssue":{"key":"PROJ-1"}`, `"outwardIssue":{"key":"PROJ-2"}`, `"comment"`, `"linked"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("link wire body missing %q:\n%s", want, body)
		}
	}
}

func TestLinkJSONInputConflictsWithToFlag(t *testing.T) {
	payload := writeJSON(t, "link.json", `{"type":{"name":"Blocks"},"inwardIssue":{"key":"PROJ-1"},"outwardIssue":{"key":"PROJ-2"}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "link", "PROJ-1", "--to", "PROJ-2", "--type", "Blocks",
		"--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("--json-input with --to/--type must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "json-input") {
		t.Fatalf("mutual-exclusion error must name the flags:\n%s", out)
	}
}

func TestLinkJSONInputInwardConflictRejected(t *testing.T) {
	payload := writeJSON(t, "link.json", `{"type":{"name":"Blocks"},"inwardIssue":{"key":"PROJ-1"},"outwardIssue":{"key":"PROJ-2"}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "link", "PROJ-9", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("a positional key disagreeing with inwardIssue must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "set twice") {
		t.Fatalf("error must explain the double inward issue: %s", out)
	}
}

func TestLinkJSONInputRejectsUnknownKeys(t *testing.T) {
	payload := writeJSON(t, "link.json", `{"type":{"name":"Blocks"},"inwardIssue":{"key":"PROJ-1"},"outwardIssue":{"key":"PROJ-2"},"commnet":{}}`)
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "link", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("an unknown body key must be rejected, not dropped:\n%s", out)
	}
	if !strings.Contains(string(out), "commnet") {
		t.Fatalf("error must name the unknown key: %s", out)
	}
}
