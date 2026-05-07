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
	"sync/atomic"
	"testing"
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

	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--json")
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

	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "linktypes.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file at %s: %v", cachePath, err)
	}

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cached second call (still 1 hit), got %d", hits.Load())
	}

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--refresh", "--json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --refresh, got %d", hits.Load())
	}

	// --ttl-minutes flag is accepted; the cache primer normalizes ttl<=0
	// to DefaultTTL inside internal/cache, so the canonical "force a
	// fetch" path is --refresh (already exercised). A high TTL against a
	// freshly-written cache should NOT trigger a new hit.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--ttl-minutes", "120", "--json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --ttl-minutes 120 against fresh cache, got %d", hits.Load())
	}

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "linktypes", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed; stat err=%v", err)
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

	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "linktypes", "--json"); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheRoot, "jira-cli", "test", "linktypes.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file missing pre-clear: %v", err)
	}
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "--json")
	if err != nil {
		t.Fatalf("cache clear: %v\n%s", err, out)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still present after `cache clear`: %v", err)
	}
}
