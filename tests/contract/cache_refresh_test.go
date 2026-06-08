package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// `jira cache refresh <resources>` refetches each resource, is TTL-gated by
// default (a fresh resource is reported "fresh" and not refetched), and
// --force refetches regardless. Statuses and priorities are flat {id,name}
// lists, so one routing mock serves both without pagination.
func TestCacheRefreshTTLGatedAndForced(t *testing.T) {
	var statusHits, priorityHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status"):
			statusHits.Add(1)
			_, _ = w.Write([]byte(`[{"id":"1","name":"Open"},{"id":"2","name":"Done"}]`))
		case strings.Contains(r.URL.Path, "/priority"):
			priorityHits.Add(1)
			_, _ = w.Write([]byte(`[{"id":"1","name":"High"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	env := append(os.Environ(), "XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"))

	// First refresh → both refetched.
	data := refreshData(t, runRefresh(t, bin, env, cfg))
	if data["succeeded"].(float64) != 2 || data["failed"].(float64) != 0 {
		t.Fatalf("first refresh: want 2 succeeded 0 failed, got %v", data)
	}
	for _, row := range data["results"].([]any) {
		m := row.(map[string]any)["data"].(map[string]any)
		if m["status"] != "refreshed" || m["from_cache"] != false {
			t.Fatalf("first refresh row not refreshed: %v", m)
		}
	}
	if statusHits.Load() != 1 || priorityHits.Load() != 1 {
		t.Fatalf("first refresh hits: status=%d priority=%d, want 1/1", statusHits.Load(), priorityHits.Load())
	}

	// Second refresh, no --force → both fresh, no new server hits.
	data = refreshData(t, runRefresh(t, bin, env, cfg))
	for _, row := range data["results"].([]any) {
		m := row.(map[string]any)["data"].(map[string]any)
		if m["status"] != "fresh" || m["from_cache"] != true {
			t.Fatalf("second refresh row should be fresh-from-cache: %v", m)
		}
	}
	if statusHits.Load() != 1 || priorityHits.Load() != 1 {
		t.Fatalf("TTL-gated refresh refetched: status=%d priority=%d, want 1/1", statusHits.Load(), priorityHits.Load())
	}

	// --force → both refetched again.
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "refresh", "statuses", "priorities", "--force", "--output=json")
	if err != nil {
		t.Fatalf("forced refresh: %v\n%s", err, out)
	}
	if statusHits.Load() != 2 || priorityHits.Load() != 2 {
		t.Fatalf("forced refresh hits: status=%d priority=%d, want 2/2", statusHits.Load(), priorityHits.Load())
	}
}

// A resource that fails to fetch must not abort the others: the envelope is
// ok:false, the healthy resource is still refreshed, and the command exits
// non-zero with the failure in errors[].
func TestCacheRefreshPartialFailureExitsNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/priority") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"1","name":"Open"}]`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	env := append(os.Environ(), "XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"))

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "refresh", "statuses", "priorities", "--output=json")
	if err == nil {
		t.Fatalf("partial failure should exit non-zero; got success\n%s", out)
	}
	// The envelope is still emitted on stdout (machine error-stream contract).
	var env1 map[string]any
	if jErr := json.Unmarshal(out, &env1); jErr != nil {
		t.Fatalf("partial-failure envelope must be valid JSON on stdout: %v\n%s", jErr, out)
	}
	if env1["ok"] != false {
		t.Fatalf("partial failure envelope ok should be false: %v", env1)
	}
	data, _ := env1["data"].(map[string]any)
	if data["succeeded"].(float64) != 1 || data["failed"].(float64) != 1 {
		t.Fatalf("want 1 succeeded 1 failed, got %v", data)
	}
	if errs, _ := env1["errors"].([]any); len(errs) != 1 {
		t.Fatalf("want one entry in errors[], got %v", env1["errors"])
	}
}

// boards stores an object (BoardsCacheFile), not a flat array, so its refresh
// goes through the bespoke prime and its count comes from the file's items.
// This exercises the r.Fetch==nil branch and the boards arm of cachedCount.
func TestCacheRefreshBoardsPath(t *testing.T) {
	var hits atomic.Int64
	srv := pagedBoardServerForCache(3, 50, &hits)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	env := append(os.Environ(), "XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"))

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "refresh", "boards", "--output=json")
	if err != nil {
		t.Fatalf("refresh boards: %v\n%s", err, out)
	}
	row := refreshData(t, out)["results"].([]any)[0].(map[string]any)
	if row["key"] != "boards" || row["ok"] != true {
		t.Fatalf("boards row: %v", row)
	}
	d := row["data"].(map[string]any)
	if d["status"] != "refreshed" || d["count"].(float64) != 3 {
		t.Fatalf("boards refresh should report 3 boards refreshed: %v", d)
	}

	// Second refresh → fresh, count still read from the cached object's items.
	out, err = runWithEnv(bin, env, "--config", cfg, "cache", "refresh", "boards", "--output=json")
	if err != nil {
		t.Fatalf("second refresh boards: %v\n%s", err, out)
	}
	d = refreshData(t, out)["results"].([]any)[0].(map[string]any)["data"].(map[string]any)
	if d["status"] != "fresh" || d["from_cache"] != true || d["count"].(float64) != 3 {
		t.Fatalf("boards second refresh should be fresh with count 3: %v", d)
	}
}

// In human/plain output a partial failure must still name the failing resource
// and exit non-zero — the failed row must not be silently dropped.
func TestCacheRefreshPlainPartialFailureNamesResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/priority") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"1","name":"Open"}]`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	env := append(os.Environ(), "XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"))

	cmd := exec.Command(bin, "--config", cfg, "cache", "refresh", "statuses", "priorities", "--output=human")
	cmd.Env = env
	combined, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("plain partial failure should exit non-zero\n%s", combined)
	}
	if !strings.Contains(string(combined), "priorities") {
		t.Fatalf("plain partial-failure output must name the failed resource 'priorities'; got:\n%s", combined)
	}
}

func runRefresh(t *testing.T, bin string, env []string, cfg string) []byte {
	t.Helper()
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "refresh", "statuses", "priorities", "--output=json")
	if err != nil {
		t.Fatalf("cache refresh: %v\n%s", err, out)
	}
	return out
}

func refreshData(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("parse refresh envelope: %v\n%s", err, out)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("refresh envelope missing data: %s", out)
	}
	return data
}
