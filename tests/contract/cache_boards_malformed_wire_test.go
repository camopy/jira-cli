// Contract test for `jira cache boards` defensive behavior on
// malformed wire data. Atlassian's API constrains both the `id` and
// `name` fields server-side, but a misconfigured proxy or future API
// shift could leak bad records — the cache primer drops them with
// structured warnings rather than letting them poison the cache.
//
// Three failure modes covered end-to-end:
//
//	bad-records-dropped     — record missing id or name (per data-model)
//	bad-project-keys-dropped — key carries JQL meta-characters
//	project-fetch-failed     — /board/{id}/project returns 5xx
package contract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// malformedBoardServer returns one valid board, one record with a
// missing id, one with an empty name, and one whose /project endpoint
// returns 500. The valid board's project endpoint returns one good key
// alongside one with an embedded comma (which would corrupt the
// emitted JQL clause if it leaked through).
func malformedBoardServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			return
		}
		body := map[string]any{
			"maxResults": 50,
			"startAt":    0,
			"isLast":     true,
			"values": []map[string]any{
				{"id": 1, "name": "Valid Board", "type": "scrum"},
				{"name": "Missing ID Board", "type": "scrum"}, // dropped
				{"id": 2, "name": "", "type": "scrum"},        // dropped
				{"id": 3, "name": "Project Fetch Fails", "type": "scrum"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, r *http.Request) {
		// /rest/agile/1.0/board/{id}/project
		switch {
		case strings.Contains(r.URL.Path, "/3/project"):
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		case strings.Contains(r.URL.Path, "/1/project"):
			body := map[string]any{
				"maxResults": 50,
				"startAt":    0,
				"isLast":     true,
				"values": []map[string]any{
					{"key": "ENG"},
					{"key": "BAD,KEY"}, // dropped — comma corrupts JQL
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
		default:
			_, _ = fmt.Fprint(w, `{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`)
		}
	})
	return httptest.NewServer(mux)
}

func TestCacheBoardsDropsMalformedRecordsWithWarnings(t *testing.T) {
	srv := malformedBoardServer()
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json")
	if err != nil {
		t.Fatalf("cache boards: %v\n%s", err, out)
	}
	var env1 map[string]any
	if err := json.Unmarshal(out, &env1); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}

	data, _ := env1["data"].(map[string]any)
	if v, _ := data["boards_count"].(float64); v != 2 {
		t.Fatalf("boards_count = %v; want 2 (valid + project-fetch-fails kept)", data["boards_count"])
	}

	warnings, _ := env1["warnings"].([]any)
	wantTypes := map[string]bool{
		"bad-records-dropped":      false,
		"bad-project-keys-dropped": false,
		"project-fetch-failed":     false,
	}
	for _, w := range warnings {
		m, _ := w.(map[string]any)
		if t, _ := m["type"].(string); t != "" {
			if _, ok := wantTypes[t]; ok {
				wantTypes[t] = true
			}
		}
	}
	for typ, seen := range wantTypes {
		if !seen {
			t.Errorf("warning type %q missing in envelope; got: %+v", typ, warnings)
		}
	}
}
