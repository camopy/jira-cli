// Contract tests for `jira cache statuses` / `jira cache priorities` and the
// matching `cachestatus` / `cachepriority` completion predictors. Both Jira
// endpoints return a flat {id,name} array, so one GET fills a cache that
// completion reads for `--status` / `--priority`.
package contract

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// namedValueServer serves a flat {id,name} array at wantPath and 404s
// elsewhere, counting hits to that path so cached reads can be asserted.
func namedValueServer(wantPath, body string, hits *atomic.Int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestCacheStatusesAndPrioritiesRoundTrip(t *testing.T) {
	cases := []struct {
		resource string
		path     string
		body     string
	}{
		{"statuses", "/rest/api/3/status", `[{"id":"1","name":"To Do"},{"id":"3","name":"In Progress"},{"id":"10001","name":"Done"}]`},
		{"priorities", "/rest/api/3/priority", `[{"id":"1","name":"Highest"},{"id":"3","name":"Medium"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			var hits atomic.Int64
			srv := namedValueServer(tc.path, tc.body, &hits)
			defer srv.Close()

			bin := buildJiraBinary(t)
			cfg := writeCacheTestConfig(t, srv.URL)
			cacheRoot := t.TempDir()
			env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

			// First call primes from the API.
			out, err := runWithEnv(bin, env, "--config", cfg, "cache", tc.resource, "--output=json")
			if err != nil {
				t.Fatalf("first cache %s: %v\n%s", tc.resource, err, out)
			}
			if hits.Load() != 1 {
				t.Fatalf("expected 1 API hit after first call, got %d", hits.Load())
			}
			data := cacheData(t, out)
			wantCount := float64(strings.Count(tc.body, `"id"`))
			if data["count"].(float64) != wantCount {
				t.Fatalf("count = %v, want %v", data["count"], wantCount)
			}
			if data["from_cache"] != false {
				t.Fatalf("first call from_cache = %v; want false", data["from_cache"])
			}
			if data["profile"] != "test" {
				t.Fatalf("profile = %v; want test", data["profile"])
			}
			values, _ := data[tc.resource].([]any)
			if len(values) != int(wantCount) {
				t.Fatalf("data.%s length = %d, want %v", tc.resource, len(values), wantCount)
			}

			cachePath := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "test", srv.URL), tc.resource+".json")
			if _, err := os.Stat(cachePath); err != nil {
				t.Fatalf("expected cache file at %s: %v", cachePath, err)
			}

			// Second call is served from cache (no new API hit).
			if _, err := runWithEnv(bin, env, "--config", cfg, "cache", tc.resource, "--output=json"); err != nil {
				t.Fatal(err)
			}
			if hits.Load() != 1 {
				t.Fatalf("expected cached second call (still 1 hit), got %d", hits.Load())
			}

			// --refresh forces a fetch even on a fresh cache.
			if _, err := runWithEnv(bin, env, "--config", cfg, "cache", tc.resource, "--refresh", "--output=json"); err != nil {
				t.Fatal(err)
			}
			if hits.Load() != 2 {
				t.Fatalf("expected 2 hits after --refresh, got %d", hits.Load())
			}

			// `cache clear <resource>` removes the file.
			if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", tc.resource, "--output=json"); err != nil {
				t.Fatalf("cache clear %s: %v", tc.resource, err)
			}
			if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
				t.Fatalf("expected cache file removed; stat err = %v", err)
			}
		})
	}
}

// The cachestatus / cachepriority predictors emit one line per unique cached
// name. The id is dropped (these back name-matched JQL filters) and per-workflow
// duplicate names collapse to one, so completion never offers "To Do" twice or
// a blank.
func TestCacheStatusPriorityPredictors(t *testing.T) {
	cases := []struct {
		predictor string
		resource  string
		fixture   string
		want      []string
	}{
		{
			// Per-workflow duplicates (To Do, In Progress) plus a blank: the
			// predictor must collapse the dups and drop the blank.
			"cachestatus", "statuses",
			`[{"id":"1","name":"To Do"},{"id":"3","name":"In Progress"},{"id":"10001","name":"Done"},` +
				`{"id":"4","name":"To Do"},{"id":"5","name":"In Progress"},{"id":"x","name":""}]`,
			[]string{"To Do", "In Progress", "Done"},
		},
		{
			"cachepriority", "priorities",
			`[{"id":"1","name":"Highest"},{"id":"3","name":"Medium"},{"id":"y","name":""}]`,
			[]string{"Highest", "Medium"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.predictor, func(t *testing.T) {
			bin := buildJiraBinary(t)
			cfg := emptyBaseURLConfig(t)
			cacheRoot := t.TempDir()

			writeNamedValueCache(t, cacheRoot, cfg, tc.resource, tc.fixture)

			c := exec.Command(bin, "--config", cfg, "--@complete="+tc.predictor, "--", "")
			c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("complete %s: %v\n%s", tc.predictor, err, out)
			}
			lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")

			// Exact set, in order: proves dedup (no repeated name) and that the
			// id is gone (a name\tid line would not equal the bare name).
			if !slices.Equal(lines, tc.want) {
				t.Fatalf("predictor %s = %q, want %q", tc.predictor, lines, tc.want)
			}
			for _, line := range lines {
				if line == "" || strings.Contains(line, "\t") {
					t.Fatalf("predictor %s emitted a blank or tab-bearing line: %q", tc.predictor, line)
				}
			}
		})
	}
}

// writeNamedValueCache primes a profile cache file for the "default" profile
// (the one --@complete resolves with emptyBaseURLConfig), mirroring the
// on-disk envelope the cache primer writes.
func writeNamedValueCache(t *testing.T, cacheRoot, cfg, resource, data string) {
	t.Helper()
	key := cacheKeyForTestConfig(t, cfg, "default", "")
	dir := filepath.Join(cacheRoot, "jira-cli", key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll cache dir: %v", err)
	}
	body := []byte(`{"profile":"` + key + `","resource":"` + resource +
		`","fetched_at":"` + time.Now().UTC().Format(time.RFC3339) + `","data":` + data + `}`)
	if err := os.WriteFile(filepath.Join(dir, resource+".json"), body, 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}
