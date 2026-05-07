// Contract tests for `jira cache boards`. Covers:
//
//	round-trip: prime → cached read → cache file present
//	--refresh bypasses TTL freshness even on a fresh cache
//	max_pages truncation surfaces in cache file + warnings
//	rate-limit-during-paginate surfaces partial-prime + warning
//
// Each test wires a small httptest server faking
// /rest/agile/1.0/board (paged) and the per-board /board/{id}/project
// endpoint that populates project_keys.
package contract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

// pagedBoardServerForCache wires both /board (paged with `total` boards
// of `pageSize`) and per-board /board/{id}/project (single project key
// derived from the board id). hits.Load() counts /board calls only —
// /board/{id}/project is incidental and excluded so tests can assert
// "API hit count" against page count alone.
func pagedBoardServerForCache(total, pageSize int, hits *atomic.Int64) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		// Path could be /board (top-level) or /board/{id}/project. Discriminate.
		if r.URL.Path != "/rest/agile/1.0/board" {
			// Per-board project lookup
			_, _ = fmt.Fprint(w, `{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"PROJ"}]}`)
			return
		}
		hits.Add(1)
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		end := min(startAt+pageSize, total)
		values := make([]map[string]any, 0, end-startAt)
		for i := startAt; i < end; i++ {
			values = append(values, map[string]any{
				"id":   i + 1,
				"self": fmt.Sprintf("http://x/board/%d", i+1),
				"name": fmt.Sprintf("Board %d", i+1),
				"type": "scrum",
			})
		}
		body := map[string]any{
			"maxResults": pageSize,
			"startAt":    startAt,
			"isLast":     end >= total,
			"values":     values,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	// Per-board project endpoint — registered under a more specific path
	// so requests like /rest/agile/1.0/board/42/project route here.
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, r *http.Request) {
		// e.g. /rest/agile/1.0/board/42/project
		body := map[string]any{
			"maxResults": 50,
			"startAt":    0,
			"isLast":     true,
			"values":     []map[string]any{{"key": "PROJ"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	return httptest.NewServer(mux)
}

func TestCacheBoardsRoundTrip(t *testing.T) {
	var hits atomic.Int64
	srv := pagedBoardServerForCache(3, 50, &hits)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// First call → primes API.
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json")
	if err != nil {
		t.Fatalf("first cache boards: %v\n%s", err, out)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 /board hit after first call, got %d", hits.Load())
	}

	var first map[string]any
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatalf("parse first envelope: %v\n%s", err, out)
	}
	data, _ := first["data"].(map[string]any)
	if data == nil {
		t.Fatalf("first envelope.data missing: %+v", first)
	}
	if v, _ := data["primed"].(bool); !v {
		t.Fatalf("first call data.primed = %v; want true", data["primed"])
	}
	if v, _ := data["boards_count"].(float64); v != 3 {
		t.Fatalf("first call data.boards_count = %v; want 3", data["boards_count"])
	}
	if v, _ := data["from_cache"].(bool); v {
		t.Fatalf("first call data.from_cache = true; want false")
	}
	if data["fetched_at"] == nil || data["fetched_at"] == "" {
		t.Fatalf("first call data.fetched_at missing: %+v", data)
	}
	if v, _ := data["truncated"].(bool); v {
		t.Fatalf("first call data.truncated = true; want false")
	}

	// Cache file present.
	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "boards.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file at %s: %v", cachePath, err)
	}

	// Second call → served from cache, no /board hit.
	out2, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json")
	if err != nil {
		t.Fatalf("second cache boards: %v\n%s", err, out2)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected /board hits to stay at 1 (cached read), got %d", hits.Load())
	}
	var second map[string]any
	if err := json.Unmarshal(out2, &second); err != nil {
		t.Fatalf("parse second envelope: %v", err)
	}
	d2, _ := second["data"].(map[string]any)
	if v, _ := d2["from_cache"].(bool); !v {
		t.Fatalf("second call data.from_cache = %v; want true", d2["from_cache"])
	}
	if v, _ := d2["primed"].(bool); v {
		t.Fatalf("second call data.primed = true; want false (cache hit)")
	}
}

func TestCacheBoardsRefreshBypassesFreshness(t *testing.T) {
	var hits atomic.Int64
	srv := pagedBoardServerForCache(2, 50, &hits)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// Prime once.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit after first prime, got %d", hits.Load())
	}

	// --refresh while cache is fresh → API IS hit.
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--refresh", "--json")
	if err != nil {
		t.Fatalf("cache boards --refresh: %v\n%s", err, out)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --refresh on fresh cache, got %d", hits.Load())
	}
	var env2 map[string]any
	if err := json.Unmarshal(out, &env2); err != nil {
		t.Fatalf("parse refresh envelope: %v", err)
	}
	d, _ := env2["data"].(map[string]any)
	if v, _ := d["primed"].(bool); !v {
		t.Fatalf("--refresh data.primed = %v; want true", d["primed"])
	}
	if v, _ := d["from_cache"].(bool); v {
		t.Fatalf("--refresh data.from_cache = true; want false")
	}
}

// Pages forever → /board never returns isLast=true. Default MaxPages=100
// should fire and write `truncated: true, truncated_reason: "max_pages"`
// to the cache file. Envelope warnings[0].type = "cache-truncated".
func TestCacheBoardsMaxPagesTruncationSurfaces(t *testing.T) {
	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			// Per-board project lookup — single project for any id.
			body := map[string]any{
				"maxResults": 50, "startAt": 0, "isLast": true,
				"values": []map[string]any{{"key": "PROJ"}},
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		hits.Add(1)
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		// Always one board per page, never isLast → forces walker into the
		// 100-page bound.
		body := map[string]any{
			"maxResults": 1,
			"startAt":    startAt,
			"isLast":     false,
			"values": []map[string]any{
				{
					"id":   startAt + 1,
					"name": fmt.Sprintf("Board %d", startAt+1),
					"type": "scrum",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"maxResults": 50, "startAt": 0, "isLast": true,
			"values": []map[string]any{{"key": "PROJ"}},
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json")
	if err != nil {
		t.Fatalf("cache boards: %v\n%s", err, out)
	}
	if hits.Load() != 100 {
		t.Fatalf("expected 100 /board hits (MaxPages bound), got %d", hits.Load())
	}

	var envOut map[string]any
	if err := json.Unmarshal(out, &envOut); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envOut["data"].(map[string]any)
	if v, _ := data["truncated"].(bool); !v {
		t.Fatalf("data.truncated = %v; want true", data["truncated"])
	}
	if v, _ := data["truncated_reason"].(string); v != "max_pages" {
		t.Fatalf("data.truncated_reason = %q; want max_pages", v)
	}
	warnings, _ := envOut["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("expected warnings[] populated, got %v", envOut["warnings"])
	}
	w0, _ := warnings[0].(map[string]any)
	if w0["type"] != "cache-truncated" {
		t.Fatalf("warnings[0].type = %v; want cache-truncated", w0["type"])
	}

	// Cache file should also persist truncated:true.
	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "boards.json")
	body, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if !bytesContains(body, `"truncated": true`) && !bytesContains(body, `"truncated":true`) {
		t.Fatalf("cache file missing truncated:true marker:\n%s", string(body))
	}
	if !bytesContains(body, `"truncated_reason": "max_pages"`) && !bytesContains(body, `"truncated_reason":"max_pages"`) {
		t.Fatalf("cache file missing truncated_reason:max_pages:\n%s", string(body))
	}
}

//	page 1 returns 200, page 2 returns 429. Walk preserves what was
//
// fetched and surfaces a rate-limit-during-paginate warning, exit 0.
func TestCacheBoardsRateLimitMidWalk(t *testing.T) {
	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			body := map[string]any{
				"maxResults": 50, "startAt": 0, "isLast": true,
				"values": []map[string]any{{"key": "PROJ"}},
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		n := hits.Add(1)
		if n == 1 {
			body := map[string]any{
				"maxResults": 1, "startAt": 0, "isLast": false,
				"values": []map[string]any{
					{"id": 1, "name": "Board 1", "type": "scrum"},
				},
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		// Page 2+ → 429 forever.
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errorMessages":["rate limited"]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"maxResults": 50, "startAt": 0, "isLast": true,
			"values": []map[string]any{{"key": "PROJ"}},
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--json")
	if err != nil {
		t.Fatalf("cache boards (expected exit 0 for partial prime): %v\n%s", err, out)
	}
	var envOut map[string]any
	if err := json.Unmarshal(out, &envOut); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envOut["data"].(map[string]any)
	if v, _ := data["truncated"].(bool); !v {
		t.Fatalf("data.truncated = %v; want true (rate-limit-mid-walk)", data["truncated"])
	}
	if v, _ := data["truncated_reason"].(string); v != "rate_limit" {
		t.Fatalf("data.truncated_reason = %q; want rate_limit", v)
	}
	warnings, _ := envOut["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("expected warnings[] populated for rate-limit-mid-walk")
	}
	w0, _ := warnings[0].(map[string]any)
	if w0["type"] != "rate-limit-during-paginate" {
		t.Fatalf("warnings[0].type = %v; want rate-limit-during-paginate", w0["type"])
	}

	// Cache file persists truncated:true / truncated_reason:rate_limit.
	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "boards.json")
	body, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if !bytesContains(body, `"truncated": true`) && !bytesContains(body, `"truncated":true`) {
		t.Fatalf("cache file missing truncated:true:\n%s", string(body))
	}
	if !bytesContains(body, `"truncated_reason": "rate_limit"`) && !bytesContains(body, `"truncated_reason":"rate_limit"`) {
		t.Fatalf("cache file missing truncated_reason:rate_limit:\n%s", string(body))
	}
}

// bytesContains is a tiny `bytes.Contains([]byte(needle))` shorthand
// kept local so the file doesn't need to import bytes for one call
// site. Renamed from `contains` to avoid colliding with the
// pre-existing `contains([]string,string)` helper in
// registry_envelope_symmetry_test.go.
func bytesContains(haystack []byte, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}
