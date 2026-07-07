// Cache linktypes primer contract — writes to linktypes.json with
// `fetched_at`; respects 60-min TTL; --refresh bypass; --ttl-minutes
// flag accepted; cache-clear primitive removes the file.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/cache"
)

func TestCacheLinkTypesRoundTrip(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issueLinkTypes":[
			{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},
			{"id":"10001","name":"Cloners","inward":"is cloned by","outward":"clones"},
			{"id":"10002","name":"Relates","inward":"relates to","outward":"relates to"}
		]}`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--output=json")
	if err != nil {
		t.Fatalf("first cache linktypes: %v\n%s", err, out)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit after first call, got %d", hits.Load())
	}
	var first map[string]any
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatalf("parse first envelope: %v\n%s", err, out)
	}
	data, _ := first["data"].(map[string]any)
	if data["count"].(float64) != 3 {
		t.Fatalf("expected count=3, got %v", data["count"])
	}
	if data["from_cache"] != false {
		t.Fatalf("first call should be from_cache=false; got %+v", data)
	}
	if data["fetched_at"] == nil || data["fetched_at"] == "" {
		t.Fatalf("data.fetched_at missing: %+v", data)
	}
	if data["profile"] != "test" {
		t.Fatalf("data.profile should echo the active profile; got %v", data["profile"])
	}
	if data["cache_state"] != "missing" || data["cache_source_state"] != "missing" || data["cache_empty"] != false {
		t.Fatalf("first call cache state wrong: %+v", data)
	}

	cachePath := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "test", srv.URL), "linktypes.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file at %s: %v", cachePath, err)
	}

	out, err = runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--output=json")
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cached second call (still 1 hit), got %d", hits.Load())
	}
	var second map[string]any
	if err := json.Unmarshal(out, &second); err != nil {
		t.Fatalf("parse second envelope: %v\n%s", err, out)
	}
	data, _ = second["data"].(map[string]any)
	if data["cache_state"] != "fresh" || data["cache_source_state"] != "fresh" || data["cache_empty"] != false {
		t.Fatalf("second call cache state wrong: %+v", data)
	}

	out, err = runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--refresh", "--output=json")
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --refresh, got %d", hits.Load())
	}
	var refreshed map[string]any
	if err := json.Unmarshal(out, &refreshed); err != nil {
		t.Fatalf("parse refresh envelope: %v\n%s", err, out)
	}
	data, _ = refreshed["data"].(map[string]any)
	if data["cache_state"] != "refresh" || data["cache_source_state"] != "refresh" || data["cache_empty"] != false {
		t.Fatalf("refresh cache state wrong: %+v", data)
	}

	// --ttl-minutes flag is accepted; the cache primer normalizes ttl<=0
	// to DefaultTTL inside internal/cache, so the canonical "force a
	// fetch" path is --refresh (already exercised). A high TTL against a
	// freshly-written cache should NOT trigger a new hit.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--ttl-minutes", "120", "--output=json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --ttl-minutes 120 against fresh cache, got %d", hits.Load())
	}

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "linktypes", "--force", "--output=json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed; stat err=%v", err)
	}
}

func TestCacheLinkTypesStatesForEmptyStaleMalformedAndCompact(t *testing.T) {
	type response struct {
		body string
	}
	responses := make(chan response, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		next := <-responses
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(next.body))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	responses <- response{body: `{"issueLinkTypes":[]}`}
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--output=json")
	if err != nil {
		t.Fatalf("empty cache linktypes: %v\n%s", err, out)
	}
	data := cacheData(t, out)
	if data["cache_state"] != "empty" || data["cache_source_state"] != "missing" || data["cache_empty"] != true {
		t.Fatalf("empty cache state wrong: %+v", data)
	}

	writeLinkTypesCacheEntry(t, cacheRoot, cfg, srv.URL, time.Now().Add(-2*time.Hour), `[{"id":"old","name":"Old"}]`)
	responses <- response{body: `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`}
	out, err = runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--ttl-minutes", "1", "--output=compact")
	if err != nil {
		t.Fatalf("stale cache linktypes: %v\n%s", err, out)
	}
	var compact map[string]any
	if err := json.Unmarshal(out, &compact); err != nil {
		t.Fatalf("parse compact data: %v\n%s", err, out)
	}
	if compact["cache_state"] != "stale" || compact["cache_source_state"] != "stale" || compact["from_cache"] != false {
		t.Fatalf("stale compact cache state wrong: %+v", compact)
	}

	writeLinkTypesCacheFile(t, cacheRoot, cfg, srv.URL, []byte(`{`))
	responses <- response{body: `{"issueLinkTypes":[{"id":"10001","name":"Relates","inward":"relates to","outward":"relates to"}]}`}
	out, err = runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--output=human")
	if err != nil {
		t.Fatalf("malformed cache linktypes: %v\n%s", err, out)
	}
	if !bytesContains(out, "cache_state") || !bytesContains(out, "malformed") {
		t.Fatalf("human output missing malformed cache state:\n%s", out)
	}
}

func cacheData(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envelope["data"].(map[string]any)
	if data == nil {
		t.Fatalf("envelope data missing: %+v", envelope)
	}
	return data
}

func writeLinkTypesCacheEntry(t *testing.T, cacheRoot, cfg, baseURL string, fetchedAt time.Time, data string) {
	t.Helper()
	body := []byte(`{
		"profile":"` + cacheKeyForTestConfig(t, cfg, "test", baseURL) + `",
		"resource":"linktypes",
		"schema":` + strconv.Itoa(cache.SchemaVersion) + `,
		"fetched_at":"` + fetchedAt.UTC().Format(time.RFC3339) + `",
		"data":` + data + `
	}`)
	writeLinkTypesCacheFile(t, cacheRoot, cfg, baseURL, body)
}

func writeLinkTypesCacheFile(t *testing.T, cacheRoot, cfg, baseURL string, body []byte) {
	t.Helper()
	path := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "test", baseURL), "linktypes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll cache dir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// `cache clear` (no arg) wipes everything in the profile, including
// linktypes.json once primed.
func TestCacheClearProfileRemovesLinkTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--output=json"); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "test", srv.URL), "linktypes.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file missing pre-clear: %v", err)
	}
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "--force", "--output=json")
	if err != nil {
		t.Fatalf("cache clear: %v\n%s", err, out)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still present after `cache clear`: %v", err)
	}
}
