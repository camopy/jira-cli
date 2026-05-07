package contract

// Every new read-only command in the boards-cache-and-default work
// MUST accept --no-input and --dry-run as no-ops without changing
// behavior. The parametric matrix below drives each command with three
// flag combinations and asserts the JSON envelope is identical to the
// unflagged baseline.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func boardsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"a-1","emailAddress":"e@x","displayName":"Me","active":true}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":42,"name":"Engineering","type":"scrum"}]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/42/project", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"ENG"}]}`))
	})
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true,"nextPageToken":""}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// stripVolatile zeros out request_id + timestamp so the same envelope
// drawn under different flag combos compares equal.
func stripVolatile(t *testing.T, body []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("envelope not JSON: %v\n%s", err, body)
	}
	if meta, ok := doc["meta"].(map[string]any); ok {
		meta["request_id"] = "<stripped>"
		meta["timestamp"] = "<stripped>"
	}
	if data, ok := doc["data"].(map[string]any); ok {
		// fetched_at is a primer-time field; varies per invocation
		// because the cache is regenerated under XDG_CACHE_HOME isolation.
		delete(data, "fetched_at")
		// from_cache flips between primer (false) and read-from-disk
		// (true) depending on whether the previous invocation populated
		// the cache file. Strip so the comparison focuses on the
		// envelope shape, not the cache state.
		delete(data, "from_cache")
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	return out
}

func TestBoardsCommandsAcceptNoInputAndDryRun(t *testing.T) {
	srv := boardsTestServer(t)
	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	// Per-test cache dir so each command invocation primes from scratch.
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	// `cache clear` is existing root-level surface from earlier work;
	// the "every NEW command" rule applies to commands introduced by
	// the boards work specifically. boards list and cache boards are
	// new; cache clear (taking "boards" as a positional arg) is
	// preexisting.
	cases := []struct {
		name string
		args []string
	}{
		{"boards.list", []string{"boards", "list"}},
		{"cache.boards", []string{"cache", "boards"}},
	}

	flagCombos := []struct {
		label string
		flags []string
	}{
		{"none", nil},
		{"no-input", []string{"--no-input"}},
		{"dry-run", []string{"--dry-run"}},
		{"no-input+dry-run", []string{"--no-input", "--dry-run"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Capture the unflagged baseline envelope; subsequent
			// invocations must match (modulo volatile fields).
			baselineCache := filepath.Join(t.TempDir(), "baseline-cache")
			t.Setenv("XDG_CACHE_HOME", baselineCache)
			baseArgs := append([]string{"--config", cfg, "--json"}, c.args...)
			baseStdout, baseStderr, baseCode := runJira(t, baseArgs...)
			if baseCode != 0 {
				t.Fatalf("baseline %v exit = %d (want 0)\nstdout=%s\nstderr=%s", c.args, baseCode, baseStdout, baseStderr)
			}
			baseline := stripVolatile(t, baseStdout)

			for _, combo := range flagCombos {
				t.Run(combo.label, func(t *testing.T) {
					comboCache := filepath.Join(t.TempDir(), "combo-cache")
					t.Setenv("XDG_CACHE_HOME", comboCache)
					args := append([]string{"--config", cfg, "--json"}, c.args...)
					args = append(args, combo.flags...)
					stdout, stderr, code := runJira(t, args...)
					if code != 0 {
						t.Fatalf("%s+%s exit = %d (want 0)\nstdout=%s\nstderr=%s", c.name, combo.label, code, stdout, stderr)
					}
					got := stripVolatile(t, stdout)
					if !bytes.Equal(baseline, got) {
						t.Fatalf("%s+%s envelope differs from baseline (no-input/dry-run policy violation)\nbaseline=%s\ngot=%s", c.name, combo.label, baseline, got)
					}
				})
			}
		})
	}

	_ = os.Stdout // keep imports balanced in case an os reference vanishes after edits
}
