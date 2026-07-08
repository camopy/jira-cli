// `alias delete` removes local state, so it carries the same headless-force
// gate as `cache clear`: a live delete off a TTY (agent / piped / --no-input)
// must consent with --force, while --dry-run stays open and writes nothing.
package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAliasGateConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"

[[profiles]]
name = "default"
base_url = "https://example.atlassian.net"
auth_type = "token"
secret_backend = "keyring"

[aliases]
mine = "issue list --assignee me"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAliasDeleteHeadlessRequiresForce(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeAliasGateConfig(t)
	env := os.Environ()

	// Headless (piped, non-TTY) delete without --force must refuse before writing.
	out, err := runWithEnv(bin, env, "--config", cfg, "--output=json", "alias", "delete", "mine")
	if err == nil || !strings.Contains(string(out), "--force") {
		t.Fatalf("headless alias delete without --force: err=%v out=%s", err, out)
	}
	if body, _ := os.ReadFile(cfg); !strings.Contains(string(body), "mine") {
		t.Fatalf("refused delete removed the alias:\n%s", body)
	}

	// --dry-run stays open, reports the would-be delete, and writes nothing.
	out, err = runWithEnv(bin, env, "--config", cfg, "--output=json", "alias", "delete", "mine", "--dry-run")
	if err != nil {
		t.Fatalf("alias delete --dry-run: %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "dry_run", true) || !envelopeHasKV(t, out, "deleted", true) {
		t.Fatalf("alias delete --dry-run output = %s", out)
	}
	if body, _ := os.ReadFile(cfg); !strings.Contains(string(body), "mine") {
		t.Fatalf("dry-run delete removed the alias:\n%s", body)
	}

	// --force consents to the live delete.
	out, err = runWithEnv(bin, env, "--config", cfg, "--output=json", "alias", "delete", "mine", "--force")
	if err != nil {
		t.Fatalf("alias delete --force: %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "deleted", true) {
		t.Fatalf("alias delete --force output = %s", out)
	}
	if body, _ := os.ReadFile(cfg); strings.Contains(string(body), "mine") {
		t.Fatalf("--force delete left the alias in config:\n%s", body)
	}
}
