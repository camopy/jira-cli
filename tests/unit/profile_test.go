package unit

import (
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestProfileValidationAndRedaction(t *testing.T) {
	cfg := config.Config{
		DefaultProfile: "default",
		Profiles: []config.Profile{{
			Name:            "default",
			BaseURL:         "https://company.atlassian.net",
			AuthType:        config.AuthTypeToken,
			Email:           "dev@example.com",
			SecretBackend:   config.SecretBackendKeyring,
			DefaultProject:  "PROJ",
			RefreshInterval: 0,
			TimeoutSeconds:  0,
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	p := cfg.Profile("default")
	if p.RefreshInterval != config.DefaultRefreshIntervalSeconds {
		t.Fatalf("RefreshInterval = %d", p.RefreshInterval)
	}
	if p.TimeoutSeconds != config.DefaultTimeoutSeconds {
		t.Fatalf("TimeoutSeconds = %d", p.TimeoutSeconds)
	}

	redacted := p.Redacted()
	if strings.Contains(redacted, "secret-token") || strings.Contains(redacted, "password-value") {
		t.Fatalf("redacted profile leaks secret language: %s", redacted)
	}
}

func TestProfileValidationRejectsDuplicateNames(t *testing.T) {
	cfg := config.Config{
		DefaultProfile: "default",
		Profiles: []config.Profile{
			{Name: "default", BaseURL: "https://a.example", AuthType: config.AuthTypeToken},
			{Name: "default", BaseURL: "https://b.example", AuthType: config.AuthTypeToken},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate name error")
	}
}

// Jira Cloud token auth is the only supported method. The previously
// supported basic/pat/mtls types — and any other value — must now be
// rejected by config validation.
func TestProfileValidationRejectsUnsupportedAuthTypes(t *testing.T) {
	for _, authType := range []string{"basic", "pat", "mtls", "oauth2"} {
		t.Run(authType, func(t *testing.T) {
			cfg := config.Config{
				DefaultProfile: "default",
				Profiles: []config.Profile{{
					Name:     "default",
					BaseURL:  "https://company.atlassian.net",
					AuthType: config.AuthType(authType),
				}},
			}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), `unsupported auth_type "`+authType+`"`) {
				t.Fatalf("Validate() with auth_type %q error = %v, want unsupported", authType, err)
			}
		})
	}
}

func twoProfileConfig() config.Config {
	return config.Config{
		DefaultProfile: "work",
		Profiles: []config.Profile{
			{Name: "work", BaseURL: "https://work.atlassian.net", AuthType: config.AuthTypeToken},
			{Name: "play", BaseURL: "https://play.atlassian.net", AuthType: config.AuthTypeToken},
		},
	}
}

// ResolveProfile: empty name resolves the configured default profile;
// a non-empty name must match a defined profile exactly; an unknown name
// errors via ErrProfileNotDefined and never fabricates a synthetic
// profile.
func TestResolveProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		wantProfile string
		wantErr     bool
		// wantNames, when set, must appear in the error message.
		wantNames []string
	}{
		{
			name:        "empty name resolves the configured default",
			input:       "",
			wantProfile: "work",
		},
		{
			name:        "exact name returns the named profile",
			input:       "play",
			wantProfile: "play",
		},
		{
			name:      "unknown name is rejected and named in the error",
			input:     "typo",
			wantErr:   true,
			wantNames: []string{"typo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := twoProfileConfig()
			p, err := cfg.ResolveProfile(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveProfile(%q) error = nil, want error", tc.input)
				}
				if !errors.Is(err, config.ErrProfileNotDefined) {
					t.Fatalf("ResolveProfile(%q) error = %v, want ErrProfileNotDefined", tc.input, err)
				}
				for _, n := range tc.wantNames {
					if !strings.Contains(err.Error(), n) {
						t.Fatalf("ResolveProfile(%q) error %q does not name %q", tc.input, err, n)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveProfile(%q) error = %v", tc.input, err)
			}
			if p.Name != tc.wantProfile {
				t.Fatalf("ResolveProfile(%q) = %q, want %q", tc.input, p.Name, tc.wantProfile)
			}
		})
	}
}

// ResolveProfile must not fabricate a profile: an unknown name yields a
// zero Profile alongside the error, not a synthetic profile carrying the
// requested name.
func TestResolveProfileDoesNotFabricate(t *testing.T) {
	t.Parallel()
	cfg := twoProfileConfig()
	p, err := cfg.ResolveProfile("ghost")
	if err == nil {
		t.Fatal("ResolveProfile(\"ghost\") error = nil, want error")
	}
	if p.Name == "ghost" {
		t.Fatalf("ResolveProfile fabricated a synthetic profile: %+v", p)
	}
}

// An empty name with no configured default profile is an error.
func TestResolveProfileErrorsWhenNoDefaultConfigured(t *testing.T) {
	t.Parallel()
	cfg := twoProfileConfig()
	cfg.DefaultProfile = ""
	if _, err := cfg.ResolveProfile(""); err == nil {
		t.Fatal("ResolveProfile(\"\") error = nil with no default profile, want error")
	}
}
