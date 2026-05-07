package unit

// `config set profiles.<name>.default_board "<anything>"` MUST succeed
// without consulting the boards cache. Validation is deferred to the
// consuming command (issue list, jql build) at use time — the cache may
// not exist yet (chicken-and-egg with `cache boards` priming).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSetDefaultBoardNoValidationAtSetTime(t *testing.T) {
	// Point cache to a temp dir to prove we don't read it during set.
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	cfg := fixtureConfig()

	// Cache is empty / missing — Set must still succeed.
	if err := cfg.Set("profiles.default.default_board", "TotallyNonexistent"); err != nil {
		t.Fatalf("Set with empty cache returned error: %v", err)
	}

	// Even an obviously fake value succeeds: no enum, no remote check,
	// no cache read.
	if err := cfg.Set("profiles.default.default_board", "🚀 Not a real board 🚀"); err != nil {
		t.Fatalf("Set with bogus value returned error: %v", err)
	}

	// Stored verbatim — Get round-trips.
	got, _ := cfg.Get("profiles.default.default_board")
	if got != "🚀 Not a real board 🚀" {
		t.Fatalf("Get = %q; want unicode-preserved value", got)
	}

	// And the cache directory should remain untouched (no file created
	// by Set; the value just lives in memory until the caller persists).
	entries, _ := os.ReadDir(filepath.Join(cacheDir))
	if len(entries) != 0 {
		t.Errorf("config Set wrote to cache dir: %v", entries)
	}
}
