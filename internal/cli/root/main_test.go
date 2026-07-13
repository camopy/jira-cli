package root

import (
	"os"
	"testing"
)

// TestMain isolates the on-disk metadata cache and the config file: this
// package's output-mode tests drive real envelope writes (which record
// recently used issue keys as a side effect), and several assembled-tree
// tests load the default config path — without the overrides a test run
// would write into the developer's real cache root, and an incompatible or
// personal ~/.config/jira-cli/config.toml (e.g. one written by a newer
// binary, which strict decode rejects) would fail unrelated tests.
func TestMain(m *testing.M) {
	cacheDir, err := os.MkdirTemp("", "jira-cli-test-cache-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CACHE_HOME", cacheDir); err != nil {
		panic(err)
	}
	cfgDir, err := os.MkdirTemp("", "jira-cli-test-config-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", cfgDir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(cacheDir)
	_ = os.RemoveAll(cfgDir)
	os.Exit(code)
}
