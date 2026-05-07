// Contract test for `jira boards list --raw`. Atlassian's native
// `/rest/agile/1.0/board` paged response shape passes through verbatim:
// `{maxResults, startAt, isLast, values}`, no envelope wrapping.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestBoardsListRawPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/agile/1.0/board" {
			_, _ = w.Write([]byte(`{
				"maxResults":50,"startAt":0,"isLast":true,"values":[
					{"id":42,"self":"https://example/board/42","name":"Engineering Sprint","type":"scrum","location":{"projectKey":"ENG"}}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--raw")
	if err != nil {
		t.Fatalf("boards list --raw: %v\n%s", err, out)
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("--raw output is not JSON: %v\n%s", err, out)
	}
	// Atlassian native shape, no envelope.
	for _, key := range []string{"maxResults", "startAt", "isLast", "values"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("--raw missing top-level %q: %+v", key, raw)
		}
	}
	if _, ok := raw["data"]; ok {
		t.Fatalf("--raw must NOT wrap in envelope (data field present): %+v", raw)
	}
	if _, ok := raw["meta"]; ok {
		t.Fatalf("--raw must NOT wrap in envelope (meta field present): %+v", raw)
	}
	values, _ := raw["values"].([]any)
	if len(values) != 1 {
		t.Fatalf("expected 1 raw value, got %d", len(values))
	}
	first := values[0].(map[string]any)
	if first["name"] != "Engineering Sprint" {
		t.Fatalf("expected verbatim Atlassian field name; got %v", first["name"])
	}
	// `location` is a server-side field that the CLI doesn't transcribe
	// into Board — `--raw` must preserve it.
	if _, ok := first["location"]; !ok {
		t.Fatalf("--raw must preserve upstream `location` field: %+v", first)
	}
}
