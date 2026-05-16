// / / `jira issue list --board NAME` and `--board-id N`
// inject `project in (P1, P2, ...)` JQL via the resolved board scope.
// The envelope MUST carry data.jql, data.board_scope, and data.precedence.
//
// --board NAME → resolves against cache, scope.applied=true,
//
//	precedence="flag", jql contains `project in (ENG, PLAT)`
//
// --board-id N  → numeric id resolution, same shape
// --board X --board-id 42 → exits non-zero (mutual exclusion)
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cache"
)

// primeBoardsCache writes a boards cache file the resolver can find.
func primeBoardsCache(t *testing.T, cacheRoot, profile, body string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	if _, err := cache.Write(profile, "boards", json.RawMessage(body)); err != nil {
		t.Fatalf("cache.Write boards: %v", err)
	}
	// Defensive: confirm the file is on disk.
	path := filepath.Join(cacheRoot, "jira-cli", profile, "boards.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected boards cache at %s: %v", path, err)
	}
}

// boardSearchServer fakes /rest/api/3/search/jql + responds with one
// stub issue. Captures every JQL it sees on jql.recv so the test can
// assert what the CLI sent.
type boardSearchServer struct {
	srv *httptest.Server
	jql chan string
}

func newBoardSearchServer(t *testing.T) *boardSearchServer {
	t.Helper()
	bss := &boardSearchServer{jql: make(chan string, 4)}
	bss.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var body struct {
			JQL string `json:"jql"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		bss.jql <- body.JQL
		_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"total":0,"issues":[]}`))
	}))
	t.Cleanup(bss.srv.Close)
	return bss
}

func TestIssueListBoardByName(t *testing.T) {
	bss := newBoardSearchServer(t)

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, bss.srv.URL)
	cacheRoot := t.TempDir()

	// Two-project board for the JQL clause emission contract.
	primeBoardsCache(t, cacheRoot, "test", `[
		{"id":42,"name":"Engineering Sprint","type":"scrum","project_keys":["ENG","PLAT"]},
		{"id":99,"name":"Other Team","type":"kanban","project_keys":["OPS"]}
	]`)
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "issue", "list", "--board", "Engineering Sprint", "--output=json")
	if err != nil {
		t.Fatalf("issue list --board: %v\n%s", err, out)
	}

	// Capture sent JQL.
	select {
	case got := <-bss.jql:
		if !strings.Contains(got, "project in (ENG, PLAT)") {
			t.Fatalf("emitted JQL missing project clause: %q", got)
		}
	default:
		t.Fatalf("expected at least one /search/jql call")
	}

	var envOut map[string]any
	if err := json.Unmarshal(out, &envOut); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envOut["data"].(map[string]any)
	if data == nil {
		t.Fatalf("data missing: %+v", envOut)
	}

	// data.jql contains the project clause.
	jql, _ := data["jql"].(string)
	if !strings.Contains(jql, "project in (ENG, PLAT)") {
		t.Fatalf("data.jql missing project clause: %q", jql)
	}

	// data.precedence = "flag"
	if v, _ := data["precedence"].(string); v != "flag" {
		t.Fatalf("data.precedence = %q; want flag", v)
	}

	// data.board_scope.applied=true, project_keys correct, name correct.
	scope, _ := data["board_scope"].(map[string]any)
	if scope == nil {
		t.Fatalf("data.board_scope missing: %+v", data)
	}
	if v, _ := scope["applied"].(bool); !v {
		t.Fatalf("board_scope.applied = false; want true")
	}
	if v, _ := scope["name"].(string); v != "Engineering Sprint" {
		t.Fatalf("board_scope.name = %q; want %q", v, "Engineering Sprint")
	}
	keys, _ := scope["project_keys"].([]any)
	if len(keys) != 2 || keys[0] != "ENG" || keys[1] != "PLAT" {
		t.Fatalf("board_scope.project_keys = %v; want [ENG PLAT]", keys)
	}
}

func TestIssueListBoardByID(t *testing.T) {
	bss := newBoardSearchServer(t)

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, bss.srv.URL)
	cacheRoot := t.TempDir()

	primeBoardsCache(t, cacheRoot, "test", `[
		{"id":42,"name":"Engineering Sprint","type":"scrum","project_keys":["ENG"]},
		{"id":99,"name":"Engineering","type":"kanban","project_keys":["OPS"]}
	]`)
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "issue", "list", "--board-id", "99", "--output=json")
	if err != nil {
		t.Fatalf("issue list --board-id: %v\n%s", err, out)
	}

	select {
	case got := <-bss.jql:
		if !strings.Contains(got, "project in (OPS)") {
			t.Fatalf("emitted JQL missing project in (OPS): %q", got)
		}
	default:
		t.Fatalf("expected /search/jql call")
	}

	var envOut map[string]any
	if err := json.Unmarshal(out, &envOut); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envOut["data"].(map[string]any)
	if v, _ := data["precedence"].(string); v != "flag" {
		t.Fatalf("data.precedence = %q; want flag", v)
	}
	scope, _ := data["board_scope"].(map[string]any)
	if scope == nil {
		t.Fatalf("data.board_scope missing")
	}
	if id, _ := scope["id"].(float64); id != 99 {
		t.Fatalf("board_scope.id = %v; want 99", scope["id"])
	}
}

func TestIssueListBoardAndBoardIDMutuallyExclusive(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()

	// Use CombinedOutput to capture both stdout + stderr; cobra prints
	// the mutual-exclusion error on stderr.
	c := exec.Command(bin, "--config", cfg, "issue", "list", "--board", "X", "--board-id", "42", "--output=json")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for mutually-exclusive flags; got success: %s", out)
	}
	if !strings.Contains(string(out), "board") || !strings.Contains(string(out), "board-id") {
		t.Fatalf("expected mutual-exclusion error mentioning --board and --board-id; got:\n%s", out)
	}
}
