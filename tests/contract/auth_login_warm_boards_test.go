package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// loginWarmServer serves the endpoints a verified login touches: /myself for
// credential verification and the agile board endpoints for the post-login
// boards-cache warm. It reports a single board.
func loginWarmServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"acc-1","displayName":"Dev User","emailAddress":"dev@example.com"}`))
	})
	// Exact "/board" → the board list; "/board/" subtree → per-board projects.
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":1,"name":"Board 1","type":"scrum"}]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"PROJ"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func warmLoginCmd(t *testing.T, bin, cfgPath, cacheRoot, baseURL string, extra ...string) *exec.Cmd {
	t.Helper()
	args := append([]string{
		"--config", cfgPath, "auth", "login", "--no-input",
		"--profile-name", "work", "--base-url", baseURL,
		"--email", "dev@example.com", "--backend", "keyring",
		"--secret-stdin", "--output=json",
	}, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	cmd.Stdin = strings.NewReader("test-token\n")
	return cmd
}

// A verified `auth login` warms the per-profile boards cache so the first
// `--board`/completion use is served from disk. The login envelope reports the
// warmed board count via boards_cached, and a subsequent `cache boards` read is
// served from that cache — proving the warm durably wrote it.
func TestAuthLoginWarmsBoardsCache(t *testing.T) {
	srv := loginWarmServer(t)
	bin := buildJiraBinary(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cacheRoot := t.TempDir()

	out, err := warmLoginCmd(t, bin, cfgPath, cacheRoot, srv.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("auth login error = %v\n%s", err, out)
	}
	var login struct {
		Data struct {
			Verified     bool `json:"verified"`
			BoardsCached *int `json:"boards_cached"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &login); err != nil {
		t.Fatalf("login output is not JSON: %v\n%s", err, out)
	}
	if !login.Data.Verified {
		t.Fatalf("login did not verify the credential:\n%s", out)
	}
	if login.Data.BoardsCached == nil || *login.Data.BoardsCached != 1 {
		t.Fatalf("boards_cached = %v, want 1\n%s", login.Data.BoardsCached, out)
	}

	// The warm must have written a usable cache: `cache boards` now serves it
	// from disk (from_cache=true) rather than refetching.
	read := exec.Command(bin, "--config", cfgPath, "cache", "boards", "--output=json")
	read.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	readOut, err := read.CombinedOutput()
	if err != nil {
		t.Fatalf("cache boards error = %v\n%s", err, readOut)
	}
	var cached struct {
		Data struct {
			FromCache   bool `json:"from_cache"`
			BoardsCount int  `json:"boards_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(readOut, &cached); err != nil {
		t.Fatalf("cache boards output is not JSON: %v\n%s", err, readOut)
	}
	if !cached.Data.FromCache || cached.Data.BoardsCount != 1 {
		t.Fatalf("cache boards served {from_cache:%v count:%d}, want a cache hit of 1 board\n%s",
			cached.Data.FromCache, cached.Data.BoardsCount, readOut)
	}
}

// A --skip-verify login stores the credential without verifying, so it must NOT
// make a network fetch to warm boards: boards_cached is absent from the envelope.
func TestAuthLoginSkipVerifyDoesNotWarmBoards(t *testing.T) {
	srv := loginWarmServer(t)
	bin := buildJiraBinary(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cacheRoot := t.TempDir()

	out, err := warmLoginCmd(t, bin, cfgPath, cacheRoot, srv.URL, "--skip-verify").CombinedOutput()
	if err != nil {
		t.Fatalf("auth login --skip-verify error = %v\n%s", err, out)
	}
	var login struct {
		Data struct {
			Verified     bool `json:"verified"`
			BoardsCached *int `json:"boards_cached"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &login); err != nil {
		t.Fatalf("login output is not JSON: %v\n%s", err, out)
	}
	if login.Data.Verified {
		t.Fatalf("--skip-verify login should not verify:\n%s", out)
	}
	if login.Data.BoardsCached != nil {
		t.Fatalf("--skip-verify login should not warm boards, got boards_cached=%d\n%s", *login.Data.BoardsCached, out)
	}
}
