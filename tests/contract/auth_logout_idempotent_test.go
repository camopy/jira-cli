package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// `auth logout` must be idempotent: removing credentials that are already
// gone is a success, not a keyring-backend failure. The keyring delete of a
// missing entry is normalized to success.
func TestAuthLogoutIsIdempotentWhenCredentialsAlreadyGone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op fixture is Unix-specific")
	}
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	// No credential was ever stored for "work"; logout must still succeed and
	// report removed=false rather than erroring on a missing keyring entry.
	cmd := exec.Command(bin, "--config", path, "--output=json", "auth", "logout", "work", "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth logout of an absent credential error = %v\n%s", err, out)
	}
	if strings.Contains(strings.ToLower(string(out)), "keyring") {
		t.Fatalf("auth logout surfaced a keyring backend error for a missing entry:\n%s", out)
	}
	if !envelopeHasKV(t, out, "removed", false) {
		t.Fatalf("auth logout of an absent credential did not report removed=false:\n%s", out)
	}
}
