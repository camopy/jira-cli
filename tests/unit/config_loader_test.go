package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// Load (the read-only / runtime view) applies the full JIRA_* env
// overlay on top of the file-backed config.
func TestLoadAppliesEnvPrecedence(t *testing.T) {
	t.Setenv("JIRA_DEFAULT_PROFILE", "env-profile")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_BASE_URL", "https://env.example.atlassian.net")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_SECRET_BACKEND", "1password")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_EMAIL", "dev@example.com")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_VAULT", "Engineering")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_ITEM", "jira-cli-env")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_REFRESH_INTERVAL", "45")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_TIMEOUT", "12")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_WORKDAY_SECONDS", "36000")

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("default_profile = \"default\"\n\n[[profiles]]\nname = \"default\"\nauth_type = \"token\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := config.Load(config.WithPath(path))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultProfile != "env-profile" {
		t.Fatalf("DefaultProfile = %q, want env-profile", cfg.DefaultProfile)
	}
	profile := cfg.Profile("env-profile")
	if got := profile.BaseURL; got != "https://env.example.atlassian.net" {
		t.Fatalf("env profile base URL = %q", got)
	}
	if profile.SecretBackend != config.SecretBackendOnePassword || profile.Email != "dev@example.com" || profile.Vault != "Engineering" || profile.Item != "jira-cli-env" || profile.RefreshInterval != 45 || profile.TimeoutSeconds != 12 || profile.WorkdaySeconds != 36000 {
		t.Fatalf("env profile overrides not applied: %+v", profile)
	}
}

// Load must not create a config file when the explicit path is missing —
// an explicit --config typo must surface as an error, not a silently
// fabricated file.
func TestLoadDoesNotCreateFileForExplicitMissingPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "typo.toml")
	if _, err := config.Load(config.WithPath(path)); err == nil {
		t.Fatal("Load() error = nil for missing explicit path, want error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Load created a config file for an explicit missing path: %v", err)
	}
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	want := filepath.Join(root, "jira-cli", "config.toml")
	if got := config.DefaultPath(); got != want {
		t.Fatalf("DefaultPath = %q, want %q", got, want)
	}
}

// Load must not create the default config file when none exists; it
// returns usable defaults without writing disk.
func TestLoadDoesNotCreateDefaultConfigFile(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil || cfg.DefaultProfile == "" {
		t.Fatalf("Load() returned unusable config: %+v", cfg)
	}
	if _, err := os.Stat(config.DefaultPath()); !os.IsNotExist(err) {
		t.Fatalf("Load created the default config file: %v", err)
	}
}

// Load must reject a config file that carries an unknown key rather than
// silently dropping it on the next Save. The error must name the
// offending key so the user can fix the typo.
func TestLoadRejectsUnknownConfigKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"
mystery_key = "oops"

[[profiles]]
name = "work"
auth_type = "token"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := config.Load(config.WithPath(path))
	if err == nil {
		t.Fatal("Load() error = nil for config with unknown key, want error")
	}
	if !strings.Contains(err.Error(), "mystery_key") {
		t.Fatalf("Load() error %q does not name the unknown key", err)
	}
}

// An unknown key nested inside a profile table must also be rejected and
// named.
func TestLoadRejectsUnknownProfileKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
auth_type = "token"
bogus_field = "nope"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := config.Load(config.WithPath(path))
	if err == nil {
		t.Fatal("Load() error = nil for profile with unknown key, want error")
	}
	if !strings.Contains(err.Error(), "bogus_field") {
		t.Fatalf("Load() error %q does not name the unknown profile key", err)
	}
}

// LoadOrInit must create parent directories and the config file when the
// default config is missing — init/write paths still bootstrap config.
func TestLoadOrInitCreatesDefaultConfigFile(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if _, err := config.LoadOrInit(); err != nil {
		t.Fatalf("LoadOrInit() error = %v", err)
	}
	if _, err := os.Stat(config.DefaultPath()); err != nil {
		t.Fatalf("LoadOrInit did not create the default config file: %v", err)
	}
}

// LoadOrInit returns the persisted, file-backed config so that callers
// that Save it do not bake transient JIRA_* env overlays into TOML.
func TestLoadOrInitOmitsEnvOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "default_profile = \"work\"\n\n[[profiles]]\nname = \"work\"\nbase_url = \"https://work.atlassian.net\"\nauth_type = \"token\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("JIRA_PROFILE_WORK_BASE_URL", "https://overlay.atlassian.net")
	t.Setenv("JIRA_DEFAULT_PROFILE", "phantom")

	cfg, err := config.LoadOrInit(config.WithPath(path))
	if err != nil {
		t.Fatalf("LoadOrInit() error = %v", err)
	}
	if cfg.DefaultProfile != "work" {
		t.Fatalf("LoadOrInit().DefaultProfile = %q, want work (env overlay leaked)", cfg.DefaultProfile)
	}
	if got := cfg.Profile("work").BaseURL; got != "https://work.atlassian.net" {
		t.Fatalf("LoadOrInit() base URL = %q, want file-backed value (env overlay leaked)", got)
	}
}

// Load still applies env overlays for the effective runtime view used by
// read-only and resolution paths.
func TestLoadAppliesEnvOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "default_profile = \"work\"\n\n[[profiles]]\nname = \"work\"\nbase_url = \"https://work.atlassian.net\"\nauth_type = \"token\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("JIRA_PROFILE_WORK_BASE_URL", "https://overlay.atlassian.net")

	cfg, err := config.Load(config.WithPath(path))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Profile("work").BaseURL; got != "https://overlay.atlassian.net" {
		t.Fatalf("Load() base URL = %q, want env-overlaid value", got)
	}
}

// Load must load and honor an existing config file.
func TestLoadReadsExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "default_profile = \"work\"\n\n[[profiles]]\nname = \"work\"\nauth_type = \"token\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := config.Load(config.WithPath(path))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DefaultProfile != "work" {
		t.Fatalf("Load().DefaultProfile = %q, want work", cfg.DefaultProfile)
	}
}
