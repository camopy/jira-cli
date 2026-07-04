package root

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gechr/clive/notify"

	"github.com/matcra587/jira-cli/internal/selfupdate"
)

// Positive control for the contract suite's notify seeding: the exact cache
// shape tests/contract/notify_test.go writes must register as a pending
// update here, otherwise its byte-clean envelope assertions pass vacuously —
// an unreadable stamp means "no update", which suppresses the hint for the
// wrong reason.
func TestSeededNotifyCacheRegistersPendingUpdate(t *testing.T) {
	t.Setenv("JIRA_NO_UPDATE_CHECK", "")
	dir := t.TempDir()
	stamp := []byte(`{"version":1,"track":"","latest":"v99.0.0"}` + "\n")
	if err := os.MkdirAll(filepath.Join(dir, "last-update"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "last-update", "check"), stamp, 0o600); err != nil {
		t.Fatal(err)
	}

	res, pending := notify.Pending(
		selfupdate.NotifyTool(),
		notify.WithCacheDir(dir),
		notify.WithCurrentVersion("v0.1.0"),
	)
	if !pending {
		t.Fatalf("seeded cache did not register as a pending update; result = %+v", res)
	}
	if res.LatestRef != "v99.0.0" {
		t.Errorf("LatestRef = %q, want %q", res.LatestRef, "v99.0.0")
	}
}
