package unit

import (
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

func TestProfileValidationRejectsUnsupportedAuthTypes(t *testing.T) {
	cfg := config.Config{
		DefaultProfile: "default",
		Profiles: []config.Profile{{
			Name:     "default",
			BaseURL:  "https://company.atlassian.net",
			AuthType: config.AuthType("oauth2"),
		}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `unsupported auth_type "oauth2"`) {
		t.Fatalf("Validate() error = %v, want unsupported oauth2 auth type", err)
	}
}
