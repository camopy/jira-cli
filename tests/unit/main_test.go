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
	os.Exit(m.Run())
}
