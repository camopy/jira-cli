// `--as-jql` is a credential-free preview path: it never calls Jira,
// so it must work even when the active profile has no usable secret
// backend (e.g. 1Password offline, keyring locked). This test pins
// that contract for the board-scope path: `--as-jql --board NAME`
// resolves the cache, emits the JQL clause, and exits 0 with the
// envelope shape — without ever touching the credential resolver.
package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIssueListAsJQLBoardScopeWorksWithoutCredentials(t *testing.T) {
	bin := buildJiraBinary(t)
	// emptyBaseURLConfig registers `secret_backend = "keyring"` with no
	// actual keyring entries — any code path that probes credentials
	// will fail. Success here proves the path is genuinely cache-only.
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()
	primeBoardsCache(t, cacheRoot, "default", `[
		{"id":42,"name":"Engineering Sprint","type":"scrum","project_keys":["ENG","PLAT"]}
	]`)

	c := exec.Command(bin, "--config", cfg, "issue", "list", "--as-jql", "--board", "Engineering Sprint", "--output=json")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --as-jql --board: %v\n%s", err, out)
	}

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatalf("envelope.data missing: %s", out)
	}
	jql, _ := data["jql"].(string)
	if !strings.Contains(jql, "project in (ENG, PLAT)") {
		t.Fatalf("data.jql missing board clause; got %q", jql)
	}
	scope, _ := data["board_scope"].(map[string]any)
	if applied, _ := scope["applied"].(bool); !applied {
		t.Fatalf("board_scope.applied = %v; want true", scope["applied"])
	}
	if precedence, _ := data["precedence"].(string); precedence != "flag" {
		t.Fatalf("data.precedence = %q; want \"flag\"", precedence)
	}
}

func TestJQLBuildBoardScopeWorksWithoutCredentials(t *testing.T) {
	// Same contract for `jql build --board NAME` — no credentials, just
	// the cache and a JQL string out.
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()
	primeBoardsCache(t, cacheRoot, "default", `[
		{"id":7,"name":"Solo Board","type":"scrum","project_keys":["SOLO"]}
	]`)

	c := exec.Command(bin, "--config", cfg, "jql", "build", "--board", "Solo Board", "--output=json")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("jql build --board: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "project in (SOLO)") {
		t.Fatalf("jql build envelope missing board clause; got:\n%s", out)
	}
}
