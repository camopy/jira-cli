package config

import "testing"

// TestAdvertisedThemeNamesValidate guards against the clib v0.5 rename: every
// name jira-cli advertises (and that users may already have in config.toml)
// must keep validating. Before the legacy-name shim, "default", "plain",
// "monochrome", and "solarized" failed here.
func TestAdvertisedThemeNamesValidate(t *testing.T) {
	for _, name := range ThemeNameValues {
		if err := ValidateThemeName(name); err != nil {
			t.Errorf("ValidateThemeName(%q) = %v, want nil", name, err)
		}
	}
}

// TestLegacyThemeNamesMapToExactVariant pins each pre-v0.5 name to the v0.5
// theme that reproduces its old palette. "solarized" in particular must map to
// the light variant (its base01 comment color), not dark.
func TestLegacyThemeNamesMapToExactVariant(t *testing.T) {
	cases := map[string]string{
		"default":    "dark",
		"plain":      "plain-dark",
		"monochrome": "monochrome-dark",
		"solarized":  "solarized-light",
	}
	for legacy, want := range cases {
		if got := ThemeForName(legacy).String(); got != want {
			t.Errorf("ThemeForName(%q) = %q, want %q", legacy, got, want)
		}
	}
}

// TestDefaultThemeHonoursEnvOverride guards the documented process-wide
// JIRA_THEME override that the v0.5 upgrade dropped from plain output and the
// TUI when theme.Default() was removed.
func TestDefaultThemeHonoursEnvOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"unset falls back to dark", "", "dark"},
		{"explicit current name", "nord", "nord"},
		{"legacy name still honored", "default", "dark"},
		{"invalid name falls back to dark", "no-such-theme", "dark"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvThemeName, tc.env)
			if got := DefaultTheme().String(); got != tc.want {
				t.Errorf("DefaultTheme() with %s=%q = %q, want %q", EnvThemeName, tc.env, got, tc.want)
			}
		})
	}
}
