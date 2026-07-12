package cmdutil

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Isolate the on-disk metadata cache: the keyed-results envelope path
	// records recently used issue keys as a side effect, and this package's
	// tests would otherwise write into the developer's real cache root.
	cacheDir, err := os.MkdirTemp("", "jira-cli-test-cache-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CACHE_HOME", cacheDir); err != nil {
		panic(err)
	}
	// goleak's VerifyTestMain calls os.Exit itself, which would skip any
	// deferred cleanup — run the leak check manually so the temp dir is
	// removed on every exit path.
	code := m.Run()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "goleak: %v\n", err)
			code = 1
		}
	}
	_ = os.RemoveAll(cacheDir)
	os.Exit(code)
}
