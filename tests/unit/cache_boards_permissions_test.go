// boards.json must be written mode 0600 (owner-only).
//
// The boards cache file must not be world-readable: it can carry
// user-visible names and project keys. The cache primer goes through
// internal/cache.Write which already produces 0600 files via
// os.CreateTemp; this test pins that invariant against a future
// regression.
package unit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/matcra587/jira-cli/internal/cache"
)

func TestBoardsCacheFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not portable to Windows")
	}
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	body, err := json.Marshal([]map[string]any{
		{"id": 1, "name": "Engineering Sprint", "type": "scrum", "project_keys": []string{"ENG"}},
	})
	if err != nil {
		t.Fatalf("marshal boards: %v", err)
	}
	if _, err := cache.Write("default", "boards", body); err != nil {
		t.Fatalf("cache.Write: %v", err)
	}

	path := filepath.Join(cacheDir, "jira-cli", "default", "boards.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat boards.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("boards.json mode = %o; want 0600", perm)
	}
}
