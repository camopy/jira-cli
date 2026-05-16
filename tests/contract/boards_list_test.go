// Contract tests for the `jira boards list` top-level read command.
//
//   - Cold-start: empty cache → first call hits the API + primes;
//     second call serves from cache (`from_cache: true`).
//   - --refresh: even a fresh cache forces a re-prime; `fetched_at`
//     advances.
//   - Truncation: a primed cache flagged `truncated: true,
//     truncated_reason: "max_pages"` surfaces `data.truncated: true`
//     plus `warnings[0].type: "cache-truncated"` carrying the named
//     limit.
//   - Empty-cache rendering: an instance with zero boards → primes,
//     returns `data.boards: []`, no error.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// boardsListServer fakes the agile board endpoint plus per-board
// project endpoints used by the cache primer. Each test wires its
// own response by setting Values / Projects.
type boardsListFake struct {
	hits  atomic.Int64
	pages atomic.Int64
}

func newBoardsServer(t *testing.T, fake *boardsListFake) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/agile/1.0/board/") && strings.HasSuffix(r.URL.Path, "/project"):
			// /rest/agile/1.0/board/{id}/project — project list
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/agile/1.0/board/"), "/project")
			switch id {
			case "42":
				_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"ENG"},{"key":"PLAT"}]}`))
			case "99":
				_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"OPS"}]}`))
			default:
				_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`))
			}
		case r.URL.Path == "/rest/agile/1.0/board":
			fake.pages.Add(1)
			_, _ = w.Write([]byte(`{
				"maxResults":50,"startAt":0,"isLast":true,"values":[
					{"id":42,"self":"https://example/board/42","name":"Engineering Sprint","type":"scrum"},
					{"id":99,"self":"https://example/board/99","name":"Ops Kanban","type":"kanban"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

// cold-start primes from API and second call serves from cache.
func TestBoardsListColdStartPrimesThenServesFromCache(t *testing.T) {
	fake := &boardsListFake{}
	srv := newBoardsServer(t, fake)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--output=json")
	if err != nil {
		t.Fatalf("first boards list: %v\n%s", err, out)
	}
	var first map[string]any
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatalf("parse first envelope: %v\n%s", err, out)
	}
	data, _ := first["data"].(map[string]any)
	if data == nil {
		t.Fatalf("envelope missing data: %+v", first)
	}
	if data["from_cache"] != false {
		t.Fatalf("first call should be from_cache=false; got %+v", data["from_cache"])
	}
	boards, _ := data["boards"].([]any)
	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d (%+v)", len(boards), boards)
	}
	// Sorted ascending by id (stable ordering).
	first0 := boards[0].(map[string]any)
	if id, _ := first0["id"].(float64); int(id) != 42 {
		t.Fatalf("expected first board id=42, got %v", first0["id"])
	}
	if name, _ := first0["name"].(string); name != "Engineering Sprint" {
		t.Fatalf("expected first board name=Engineering Sprint, got %q", name)
	}
	pks, _ := first0["project_keys"].([]any)
	if len(pks) != 2 {
		t.Fatalf("expected 2 project_keys, got %v", first0["project_keys"])
	}
	// Pagination is always present ( shape consistency).
	if _, ok := data["pagination"].(map[string]any); !ok {
		t.Fatalf("data.pagination missing: %+v", data)
	}
	if data["fetched_at"] == nil || data["fetched_at"] == "" {
		t.Fatalf("data.fetched_at missing: %+v", data)
	}

	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "boards.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file at %s: %v", cachePath, err)
	}

	// Second call → from cache, no additional hits to /board.
	pagesBefore := fake.pages.Load()
	out2, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--output=json")
	if err != nil {
		t.Fatalf("second boards list: %v\n%s", err, out2)
	}
	var second map[string]any
	if err := json.Unmarshal(out2, &second); err != nil {
		t.Fatalf("parse second envelope: %v", err)
	}
	data2, _ := second["data"].(map[string]any)
	if data2["from_cache"] != true {
		t.Fatalf("second call should be from_cache=true; got %+v", data2["from_cache"])
	}
	if got := fake.pages.Load(); got != pagesBefore {
		t.Fatalf("expected no additional /board hits on second call (was %d, now %d)", pagesBefore, got)
	}
}

// --refresh forces an API hit even when the cache is fresh.
func TestBoardsListRefreshForcesPrime(t *testing.T) {
	fake := &boardsListFake{}
	srv := newBoardsServer(t, fake)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// Prime once.
	if _, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--output=json"); err != nil {
		t.Fatal(err)
	}
	pagesAfterFirst := fake.pages.Load()
	if pagesAfterFirst != 1 {
		t.Fatalf("expected 1 /board hit after first prime, got %d", pagesAfterFirst)
	}

	// --refresh → hits /board again.
	out, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--refresh", "--output=json")
	if err != nil {
		t.Fatalf("--refresh: %v\n%s", err, out)
	}
	if got := fake.pages.Load(); got != pagesAfterFirst+1 {
		t.Fatalf("expected /board hit count to advance to %d after --refresh, got %d", pagesAfterFirst+1, got)
	}
	var ref map[string]any
	if err := json.Unmarshal(out, &ref); err != nil {
		t.Fatalf("parse --refresh envelope: %v\n%s", err, out)
	}
	data, _ := ref["data"].(map[string]any)
	if data["from_cache"] != false {
		t.Fatalf("--refresh should set from_cache=false; got %+v", data["from_cache"])
	}
}

// a cache file flagged truncated surfaces a `cache-truncated`
// warning + `data.truncated: true` and `data.truncated_reason` set.
func TestBoardsListSurfacesTruncationWarning(t *testing.T) {
	// httptest server exists only so the config validates as
	// loopback HTTP; the cache is fresh, so the command never
	// touches the wire.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Errorf("server should not be hit when cache is fresh")
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// Pre-write a cache file that has truncation metadata so the
	// command serves from cache and surfaces the warning without
	// hitting the network.
	cacheDir := filepath.Join(cacheRoot, "jira-cli", "test")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, "boards.json")
	// Use the current time so the cache is fresh regardless of the
	// test machine's clock. Far-future TTL also keeps the file from
	// being considered stale.
	fetched := time.Now().UTC().Format(time.RFC3339)
	body := `{
		"profile":"test",
		"resource":"boards",
		"fetched_at":"` + fetched + `",
		"data": {
			"items":[{"id":42,"name":"Eng","type":"scrum","project_keys":["ENG"]}],
			"truncated": true,
			"truncated_reason": "max_pages"
		}
	}`
	if err := os.WriteFile(cachePath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--output=json", "--ttl-minutes", "9999")
	if err != nil {
		t.Fatalf("boards list (truncated cache): %v\n%s", err, out)
	}
	var envel map[string]any
	if err := json.Unmarshal(out, &envel); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envel["data"].(map[string]any)
	if data["truncated"] != true {
		t.Fatalf("data.truncated should be true; got %+v", data["truncated"])
	}
	if data["truncated_reason"] != "max_pages" {
		t.Fatalf("data.truncated_reason should be max_pages; got %v", data["truncated_reason"])
	}
	warnings, _ := envel["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("expected warnings; got %+v", envel["warnings"])
	}
	w0, _ := warnings[0].(map[string]any)
	if w0["type"] != "cache-truncated" {
		t.Fatalf("warnings[0].type should be cache-truncated; got %v", w0["type"])
	}
	msg, _ := w0["message"].(string)
	if !strings.Contains(msg, "max_pages") {
		t.Fatalf("warning message should name the limit (max_pages); got %q", msg)
	}
}

// empty-cache rendering — instance with zero boards visible →
// `data.boards: []`, no error.
func TestBoardsListEmptyInstanceReturnsEmptyArray(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/agile/1.0/board" {
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "boards", "list", "--output=json")
	if err != nil {
		t.Fatalf("empty-instance boards list: %v\n%s", err, out)
	}
	var envel map[string]any
	if err := json.Unmarshal(out, &envel); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envel["data"].(map[string]any)
	boards, ok := data["boards"].([]any)
	if !ok {
		t.Fatalf("data.boards must be an array (even when empty); got %T %+v", data["boards"], data["boards"])
	}
	if len(boards) != 0 {
		t.Fatalf("expected 0 boards, got %d", len(boards))
	}
	if errs, _ := envel["errors"].([]any); len(errs) != 0 {
		t.Fatalf("expected no errors on empty-instance list; got %+v", errs)
	}
}
