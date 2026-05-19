package unit

import (
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// fixtureConfig returns a Config with two profiles for Get/Set exercising.
func fixtureConfig() *config.Config {
	cfg := config.Defaults()
	cfg.Profiles = []config.Profile{
		{
			Name:            "default",
			BaseURL:         "https://example.atlassian.net",
			AuthType:        config.AuthTypeToken,
			Email:           "dev@example.com",
			SecretBackend:   config.SecretBackendKeyring,
			RefreshInterval: 30,
			TimeoutSeconds:  30,
			WorkdaySeconds:  28800,
		},
		{
			Name:            "work",
			BaseURL:         "https://work.atlassian.net",
			AuthType:        config.AuthTypePAT,
			SecretBackend:   config.SecretBackendOnePassword,
			RefreshInterval: 30,
			TimeoutSeconds:  30,
			WorkdaySeconds:  28800,
		},
	}
	return &cfg
}

func TestConfigGet_TopLevel(t *testing.T) {
	cfg := fixtureConfig()
	cfg.DefaultProfile = "default"
	cfg.QueriesPath = "/tmp/queries"
	cfg.Editor = "nvim"
	cfg.Theme.Name = "dracula"
	cfg.TUI.RefreshInterval = 45
	cfg.TUI.DefaultTab = "search"

	cases := map[string]string{
		"default_profile":      "default",
		"queries_path":         "/tmp/queries",
		"editor":               "nvim",
		"theme.name":           "dracula",
		"tui.refresh_interval": "45",
		"tui.default_tab":      "search",
	}
	for key, want := range cases {
		got, ok := cfg.Get(key)
		if !ok {
			t.Errorf("Get(%q) ok=false", key)
			continue
		}
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestConfigGet_ProfileScoped(t *testing.T) {
	cfg := fixtureConfig()
	cases := map[string]string{
		"profiles.default.base_url":         "https://example.atlassian.net",
		"profiles.default.auth_type":        "token",
		"profiles.default.email":            "dev@example.com",
		"profiles.default.secret_backend":   "keyring",
		"profiles.default.refresh_interval": "30",
		"profiles.default.read_only":        "false",
		"profiles.work.auth_type":           "pat",
		"profiles.work.secret_backend":      "1password",
	}
	for key, want := range cases {
		got, ok := cfg.Get(key)
		if !ok {
			t.Errorf("Get(%q) ok=false", key)
			continue
		}
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestConfigGet_UnknownKeyReturnsFalse(t *testing.T) {
	cfg := fixtureConfig()
	for _, key := range []string{
		"nonsense",
		"profiles.missing.base_url",
		"profiles.default.nope",
	} {
		if _, ok := cfg.Get(key); ok {
			t.Errorf("Get(%q) ok=true, want false", key)
		}
	}
}

func TestConfigSet_ProfileScopedFlipsBackend(t *testing.T) {
	cfg := fixtureConfig()
	if err := cfg.Set("profiles.default.secret_backend", "1password"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	got, _ := cfg.Get("profiles.default.secret_backend")
	if got != "1password" {
		t.Fatalf("Get after Set = %q, want 1password", got)
	}
	// The other profile is untouched.
	if cfg.Profile("work").SecretBackend != config.SecretBackendOnePassword {
		t.Fatal("work profile mutated when only default should change")
	}
}

func TestConfigSet_RejectsInvalidEnum(t *testing.T) {
	cfg := fixtureConfig()
	cases := map[string]string{
		"profiles.default.auth_type":      "garbage",
		"profiles.default.secret_backend": "vault",
		"profiles.default.read_only":      "maybe",
		"tui.default_tab":                 "nope",
	}
	for key, value := range cases {
		err := cfg.Set(key, value)
		if err == nil {
			t.Errorf("Set(%q, %q) error = nil, want validation error", key, value)
		}
	}
}

func TestConfigSet_RejectsInvalidInt(t *testing.T) {
	cfg := fixtureConfig()
	cases := map[string]string{
		"profiles.default.refresh_interval": "zero",
		"profiles.default.timeout":          "-5",
		"tui.refresh_interval":              "lots",
	}
	for key, value := range cases {
		if err := cfg.Set(key, value); err == nil {
			t.Errorf("Set(%q, %q) error = nil, want validation error", key, value)
		}
	}
}

func TestConfigSet_UnknownKeyOrProfile(t *testing.T) {
	cfg := fixtureConfig()
	cases := map[string]string{
		"nonsense":                  "x",
		"profiles.missing.base_url": "https://x",
		"profiles.default.nope":     "x",
	}
	for key, value := range cases {
		if err := cfg.Set(key, value); err == nil {
			t.Errorf("Set(%q, %q) error = nil, want unknown error", key, value)
		}
	}
}

func TestKeys_ExpandsAcrossProfiles(t *testing.T) {
	cfg := fixtureConfig()
	keys := config.Keys(cfg)

	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.Name
	}

	// Top-level samples.
	for _, want := range []string{
		"default_profile",
		"editor",
		"tui.default_tab",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("Keys missing top-level %q", want)
		}
	}

	// Both profiles expanded.
	for _, want := range []string{
		"profiles.default.secret_backend",
		"profiles.default.auth_type",
		"profiles.work.secret_backend",
		"profiles.work.email",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("Keys missing profile-scoped %q", want)
		}
	}
}

func TestKeyChoices_Enums(t *testing.T) {
	cases := map[string][]string{
		"profiles.default.auth_type":      {"token", "basic", "pat", "mtls"},
		"profiles.default.secret_backend": {"keyring", "1password"},
		"profiles.default.read_only":      {"true", "false"},
		"theme.name":                      config.ThemeNameValues,
		"tui.default_tab":                 {"issues", "epics", "search", "activity"},
	}
	for key, want := range cases {
		got := config.KeyChoices(key)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("KeyChoices(%q) = %v, want %v", key, got, want)
		}
	}

	if got := config.KeyChoices("profiles.default.email"); got != nil {
		t.Errorf("KeyChoices for freeform key returned %v, want nil", got)
	}
}
