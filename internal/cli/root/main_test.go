package root

import (
	"os"
	"testing"
)

// TestMain isolates the on-disk metadata cache: this package's output-mode
// tests drive real envelope writes, which record recently used issue keys
// as a side effect — without the override every test run would write into
// the developer's real cache root.
func TestMain(m *testing.M) {
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
