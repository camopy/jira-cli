package auth

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/config"
)

// Configuring a second profile must never drop the first. This runs login the
// way production runs it — no --config flag, so the path resolves to the
// default location — because the wipe only triggered on that path: the
// fresh-config probe stat'ed the raw (empty) flag value instead of the
// resolved default path, decided the config was new, and cleared every loaded
// profile before the save.
func TestAuthLoginPreservesExistingProfiles(t *testing.T) {
	// Redirect the default config root into the test's sandbox; DefaultPath()
	// resolves under XDG_CONFIG_HOME on Unix.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	seeded := config.Config{
		DefaultProfile: "alpha",
		Profiles: []config.Profile{{
			Name:          "alpha",
			BaseURL:       "https://alpha.example.com",
			Email:         "alpha@example.com",
			AuthType:      config.AuthTypeToken,
			SecretBackend: config.SecretBackendKeyring,
		}},
	}
	if err := config.Save(config.DefaultPath(), &seeded); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	// Empty configPath = the production default: the --config flag unset.
	_, stderr, err := runAuthLoginInProcess(
		t, cli.ModeJSON, "",
		"--profile-name", "beta",
		"--base-url", "https://beta.example.com",
		"--email", "beta@example.com",
		"--backend", "keyring",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}

	cfg, err := config.Load(config.WithPath(config.DefaultPath()))
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	byName := make(map[string]config.Profile, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		byName[p.Name] = p
	}
	if len(cfg.Profiles) != 2 {
		names := make([]string, 0, len(cfg.Profiles))
		for _, p := range cfg.Profiles {
			names = append(names, p.Name)
		}
		t.Fatalf("profiles after second login = %v, want [alpha beta]", names)
	}
	alpha, ok := byName["alpha"]
	if !ok {
		t.Fatalf("existing profile alpha was dropped by the second login")
	}
	if alpha.BaseURL != "https://alpha.example.com" || alpha.Email != "alpha@example.com" {
		t.Errorf("existing profile alpha mutated: base_url=%q email=%q", alpha.BaseURL, alpha.Email)
	}
	beta, ok := byName["beta"]
	if !ok {
		t.Fatalf("new profile beta was not saved")
	}
	if beta.BaseURL != "https://beta.example.com" {
		t.Errorf("new profile beta base_url = %q, want https://beta.example.com", beta.BaseURL)
	}
	if cfg.DefaultProfile != "beta" {
		t.Errorf("default profile = %q, want beta (login switches the default)", cfg.DefaultProfile)
	}
}

// A login that CREATES the config file must still start from exactly the
// requested profile — no phantom "default" seed beside it. This pins the
// behavior the fresh-config gate exists for, now keyed off the resolved path.
func TestAuthLoginFreshConfigCreatesOnlyRequestedProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, stderr, err := runAuthLoginInProcess(
		t, cli.ModeJSON, "",
		"--profile-name", "work",
		"--base-url", "https://work.example.com",
		"--email", "work@example.com",
		"--backend", "keyring",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}

	cfg, err := config.Load(config.WithPath(config.DefaultPath()))
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != "work" {
		names := make([]string, 0, len(cfg.Profiles))
		for _, p := range cfg.Profiles {
			names = append(names, p.Name)
		}
		t.Fatalf("profiles after fresh-config login = %v, want [work] only", names)
	}
}
