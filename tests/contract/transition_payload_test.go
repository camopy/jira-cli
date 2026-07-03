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

// `issue transition` carries an optional payload applied atomically with
// the status change: a Markdown comment (--markdown / --markdown-file) and
// field updates via --json-input — the transition screen is where fields
// like resolution live, so a close-with-resolution is one call.

func TestTransitionDryRunPreviewsCommentAndFields(t *testing.T) {
	payload := filepath.Join(t.TempDir(), "transition.json")
	if err := os.WriteFile(payload, []byte(`{"fields":{"resolution":{"name":"Done"}},"comment":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"closing note"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"issue", "transition", "PROJ-1", "Done", "--dry-run", "--no-input",
		"--json-input", payload, "--output=json").Output()
	if err != nil {
		t.Fatalf("transition dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Transition string         `json:"transition"`
			Fields     map[string]any `json:"fields"`
			Comment    map[string]any `json:"comment"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %v\n%s", jerr, out)
	}
	if env.Data.Transition != "Done" {
		t.Fatalf("preview must echo the target: %s", out)
	}
	if _, ok := env.Data.Fields["resolution"]; !ok {
		t.Fatalf("preview must carry the validated fields: %s", out)
	}
	if env.Data.Comment["type"] != "doc" {
		t.Fatalf("preview must carry the parsed comment document: %s", out)
	}
}

func TestTransitionMarkdownCommentConverts(t *testing.T) {
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"issue", "transition", "PROJ-1", "Done", "--dry-run", "--no-input",
		"--markdown", "released in **v1.2.3**", "--output=json").Output()
	if err != nil {
		t.Fatalf("transition --markdown dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"strong"`) {
		t.Fatalf("Markdown comment must convert to ADF with marks: %s", out)
	}
}

func TestTransitionPayloadRequiresTarget(t *testing.T) {
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"issue", "transition", "PROJ-1", "--dry-run", "--no-input",
		"--markdown", "note without a status", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("a payload without a target status must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "target status") {
		t.Fatalf("error must explain the payload needs a target: %s", out)
	}
}

func TestTransitionMarkdownExcludesJSONInput(t *testing.T) {
	payload := filepath.Join(t.TempDir(), "t.json")
	if err := os.WriteFile(payload, []byte(`{"fields":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"issue", "transition", "PROJ-1", "Done", "--dry-run", "--no-input",
		"--markdown", "x", "--json-input", payload, "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("--markdown with --json-input must be rejected:\n%s", out)
	}
	if !strings.Contains(string(out), "markdown") || !strings.Contains(string(out), "json-input") {
		t.Fatalf("mutual-exclusion error must name both flags:\n%s", out)
	}
}

// A screened transition submits the payload; the wire body carries the
// fields and the update.comment block Jira applies atomically.
func TestTransitionWithScreenSubmitsPayload(t *testing.T) {
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

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "transition", "PROJ-1", "Done", "--no-input",
		"--markdown", "closing note", "--output=json")
	if code != 0 {
		t.Fatalf("screened transition with payload must succeed, exit=%d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	body := string(posted)
	for _, want := range []string{`"transition":{"id":"31"}`, `"update"`, `"comment"`, `"closing note"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("transition body missing %q:\n%s", want, body)
		}
	}
}

// A screenless transition refuses the payload outright: Jira would accept
// the request and silently discard the fields and comment — the silent
// loss this CLI exists to prevent.
func TestTransitionWithoutScreenRefusesPayload(t *testing.T) {
	var posts int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"Done","hasScreen":false}]}`))
	})
	mux.HandleFunc("POST /rest/api/3/issue/PROJ-1/transitions", func(w http.ResponseWriter, _ *http.Request) {
		posts++
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "transition", "PROJ-1", "Done", "--no-input",
		"--markdown", "would be discarded", "--output=json")
	if code == 0 {
		t.Fatalf("screenless transition with payload must be refused\nstdout=%s", stdout)
	}
	if posts != 0 {
		t.Fatalf("nothing may reach the wire after the refusal, got %d POSTs", posts)
	}
	combined := string(stdout) + string(stderr)
	if !strings.Contains(combined, "no screen") || !strings.Contains(combined, "issue comment add") {
		t.Fatalf("refusal must explain the discard and the recovery:\n%s", combined)
	}
}
