// `cache clear boards` removes the per-profile boards.json file.
// Idempotent: calling it again on a missing file still exits 0.
package contract

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheClearBoardsRemovesFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"PROJ"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":1,"name":"Board 1","type":"scrum"}]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"PROJ"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// Prime the cache.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "boards.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file should exist pre-clear: %v", err)
	}

	// Clear → file removed, exit 0.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "boards", "--json"); err != nil {
		t.Fatalf("cache clear boards: %v", err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still present after clear: %v", err)
	}

	// Idempotent: clear again → exit 0, no error.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "boards", "--json"); err != nil {
		t.Fatalf("idempotent cache clear boards: %v", err)
	}
}
