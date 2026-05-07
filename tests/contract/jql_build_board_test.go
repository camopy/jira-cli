// `jql build --board NAME` emits the same project clause and
// envelope additions (board_scope + precedence) as `issue list --board`.
package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestJQLBuildBoardByName(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()
	primeBoardsCache(t, cacheRoot, "default", `[
		{"id":7,"name":"Engineering Sprint","type":"scrum","project_keys":["ENG","PLAT"]}
	]`)
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "jql", "build", "--board", "Engineering Sprint", "--status", "Open", "--json")
	if err != nil {
		t.Fatalf("jql build --board: %v\n%s", err, out)
	}
	var envOut map[string]any
	if err := json.Unmarshal(out, &envOut); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envOut["data"].(map[string]any)
	if data == nil {
		t.Fatalf("data missing: %+v", envOut)
	}

	jql, _ := data["jql"].(string)
	if !strings.Contains(jql, "project in (ENG, PLAT)") {
		t.Fatalf("data.jql missing project clause: %q", jql)
	}
	if v, _ := data["precedence"].(string); v != "flag" {
		t.Fatalf("data.precedence = %q; want flag", v)
	}
	scope, _ := data["board_scope"].(map[string]any)
	if scope == nil || scope["applied"] != true {
		t.Fatalf("data.board_scope missing or not applied: %+v", data)
	}
	if v, _ := scope["name"].(string); v != "Engineering Sprint" {
		t.Fatalf("board_scope.name = %q; want Engineering Sprint", v)
	}
}
