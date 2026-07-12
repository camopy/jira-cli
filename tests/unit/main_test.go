package unit

import (
	"os"
	"strings"
	"testing"
)

// TestMain clears every JIRA_TOKEN_* credential override so credential-status
// tests are deterministic regardless of the ambient environment. The override
// is checked before the profile's configured backend for every backend, so a
// developer shell exporting JIRA_TOKEN_DEFAULT would otherwise outrank the
// fixture stores for any test profile named "default".
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "JIRA_TOKEN_") {
			continue
		}
		if err := os.Unsetenv(name); err != nil {
			panic(err)
		}
	}
	// Isolate the on-disk metadata cache: commands record recently used
	// issue keys as a side effect, and in-process command tests would
	// otherwise write into the developer's real cache root.
	cacheDir, err := os.MkdirTemp("", "jira-cli-test-cache-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CACHE_HOME", cacheDir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(cacheDir)
	os.Exit(code)
}
