package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestLoadCreatesDefaultAndAppliesEnvPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("JIRA_DEFAULT_PROFILE", "env-profile")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_BASE_URL", "https://env.example.atlassian.net")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_AUTH_TYPE", "pat")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_SECRET_BACKEND", "1password")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_EMAIL", "dev@example.com")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_USERNAME", "dev")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_VAULT", "Engineering")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_ITEM", "jira-cli-env")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_REFRESH_INTERVAL", "45")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_TIMEOUT", "12")
	t.Setenv("JIRA_PROFILE_ENV_PROFILE_WORKDAY_SECONDS", "36000")

	path := filepath.Join(t.TempDir(), "config.toml")
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
	if profile.AuthType != config.AuthTypePAT || profile.SecretBackend != config.SecretBackendOnePassword || profile.Email != "dev@example.com" || profile.Username != "dev" || profile.Vault != "Engineering" || profile.Item != "jira-cli-env" || profile.RefreshInterval != 45 || profile.TimeoutSeconds != 12 || profile.WorkdaySeconds != 36000 {
		t.Fatalf("env profile overrides not applied: %+v", profile)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default config was not created: %v", err)
	}
}
