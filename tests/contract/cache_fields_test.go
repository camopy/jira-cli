package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// `jira cache fields` MUST round-trip the visible Jira field list
// through the local cache, supporting --refresh and --ttl-minutes.
func TestCacheFieldsRoundTrip(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[
			{"id":"summary","name":"Summary","schema":{"type":"string"}},
			{"id":"customfield_10001","name":"Story Points","schema":{"type":"number"}}
		]`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// First call → fetches.
	out, err := runWithEnv(bin, env, "--config", cfg, "cache", "fields", "--output=json")
	if err != nil {
		t.Fatalf("first fields call: %v\n%s", err, out)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 server hit, got %d", hits.Load())
	}
	var env1 map[string]any
	if err := json.Unmarshal(out, &env1); err != nil {
		t.Fatalf("parse first envelope: %v\n%s", err, out)
	}
	data1, _ := env1["data"].(map[string]any)
	if data1["count"].(float64) != 2 {
		t.Fatalf("expected 2 fields, got %v", data1["count"])
	}

	// Second call → cached.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "fields", "--output=json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cached second call (still 1 hit), got %d", hits.Load())
	}

	// --refresh → fetches again.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "fields", "--refresh", "--output=json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 server hits after --refresh, got %d", hits.Load())
	}
}

func writeCacheTestConfig(t *testing.T, baseURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "default_profile = \"test\"\n\n[[profiles]]\nname = \"test\"\nbase_url = \"" + baseURL + "\"\nauth_type = \"token\"\nemail = \"u@example.com\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func runWithEnv(bin string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	return cmd.Output()
}
