// `cache clear boards` removes the per-profile boards.json file.
// Idempotent: calling it again on a missing file still exits 0.
package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "boards", "--output=json"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	cachePath := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "test", srv.URL), "boards.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file should exist pre-clear: %v", err)
	}

	// Clear → file removed, exit 0.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "boards", "--output=json"); err != nil {
		t.Fatalf("cache clear boards: %v", err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still present after clear: %v", err)
	}

	// Idempotent: clear again → exit 0, no error.
	if _, err := runWithEnv(bin, env, "--config", cfg, "cache", "clear", "boards", "--output=json"); err != nil {
		t.Fatalf("idempotent cache clear boards: %v", err)
	}
}

// `cache clear --profile <typo>` must fail before touching any cache:
// it must not fall back to deleting the default profile's cache files.
func TestCacheClearRejectsUnknownExplicitProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeTwoProfileConfig(t)
	cacheRoot := t.TempDir()

	// Seed a cache file under the *real* "work" profile so we can prove
	// the typoed run leaves it untouched.
	workCache := filepath.Join(cacheRoot, "jira-cli", cacheKeyForTestConfig(t, cfg, "work", ""))
	if err := os.MkdirAll(workCache, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seeded := filepath.Join(workCache, "projects.json")
	if err := os.WriteFile(seeded, []byte("[]"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	c := exec.Command(bin, "--config", cfg, "--profile", "typo", "cache", "clear", "--output=json")
	c.Env = env
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err == nil {
		t.Fatalf("cache clear with typoed profile succeeded:\nstdout=%s", stdout.String())
	}
	if _, err := os.Stat(seeded); err != nil {
		t.Fatalf("cache clear with typoed profile deleted the work-profile cache: %v", err)
	}
}

func TestCacheClearRejectsUnknownResource(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, "https://example.atlassian.net")
	env := []string{"XDG_CACHE_HOME=" + t.TempDir()}

	envelope, runErr := requireEnvelopeOnStdoutWithEnv(
		t,
		bin,
		env,
		"--config", cfg,
		"cache", "clear", "bogus",
		"--output=json",
	)
	assertValidationExitCode(t, runErr)
	if ok, _ := envelope["ok"].(bool); ok {
		t.Fatalf("ok = true, want false: %#v", envelope)
	}
	errorsOut, ok := envelope["errors"].([]any)
	if !ok || len(errorsOut) == 0 {
		t.Fatalf("errors = %#v, want at least one validation error", envelope["errors"])
	}
	first, ok := errorsOut[0].(map[string]any)
	if !ok {
		t.Fatalf("first error = %#v, want object", errorsOut[0])
	}
	if first["type"] != "validation" {
		t.Fatalf("error type = %#v, want validation", first["type"])
	}
	if first["code"] != "arg_value_invalid" {
		t.Fatalf("error code = %#v, want arg_value_invalid", first["code"])
	}
	message, _ := first["message"].(string)
	if !strings.Contains(message, `unknown cache resource "bogus"`) {
		t.Fatalf("error message %q does not name the bad resource", message)
	}
	for _, resource := range []string{"labels", "projects", "epics", "fields", "issuetypes", "linktypes", "boards", "statuses", "priorities"} {
		if !strings.Contains(message, resource) {
			t.Fatalf("error message %q does not name valid resource %q", message, resource)
		}
	}
}
