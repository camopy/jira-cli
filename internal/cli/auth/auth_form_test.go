package auth

import (
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// On a re-login, the interactive form must pre-fill from the persisted
// profile so the user edits current values instead of retyping everything.
// A flag the user passed explicitly wins over the persisted value.
func TestApplyLoginPreseedFillsFromExistingProfile(t *testing.T) {
	profile := config.Profile{
		BaseURL:            "https://old.atlassian.net",
		Email:              "old@example.com",
		SecretBackend:      config.SecretBackendOnePassword,
		OnePasswordAccount: "my.1password.com",
		Vault:              "Engineering",
		Item:               "jira-cli-work",
	}
	// backend starts at its flag default ("keyring") but was not explicitly
	// changed, so the persisted 1password backend must win.
	baseURL, email, backend := "", "", "keyring"
	opAccount, vault, item := "", "", ""

	applyLoginPreseed(profile, func(string) bool { return false },
		&baseURL, &email, &backend, &opAccount, &vault, &item)

	for _, tc := range []struct{ got, want, field string }{
		{baseURL, "https://old.atlassian.net", "base_url"},
		{email, "old@example.com", "email"},
		{backend, string(config.SecretBackendOnePassword), "backend"},
		{opAccount, "my.1password.com", "onepassword_account"},
		{vault, "Engineering", "vault"},
		{item, "jira-cli-work", "item"},
	} {
		if tc.got != tc.want {
			t.Errorf("preseed %s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

func TestApplyLoginPreseedRespectsExplicitFlags(t *testing.T) {
	profile := config.Profile{BaseURL: "https://old.atlassian.net", Email: "old@example.com"}
	baseURL, email := "https://new.atlassian.net", ""
	backend, opAccount, vault, item := "keyring", "", "", ""

	// base-url was passed explicitly; email was not.
	applyLoginPreseed(profile, func(name string) bool { return name == "base-url" },
		&baseURL, &email, &backend, &opAccount, &vault, &item)

	if baseURL != "https://new.atlassian.net" {
		t.Errorf("explicit base-url was overwritten by preseed: %q", baseURL)
	}
	if email != "old@example.com" {
		t.Errorf("email was not preseeded from the existing profile: %q", email)
	}
}

func TestAuthLoginPreseedProfileUsesConfiguredDefaultWhenProfileNameWasImplicit(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: []config.Profile{
			{
				Name:          "work",
				BaseURL:       "https://work.atlassian.net",
				Email:         "work@example.com",
				SecretBackend: config.SecretBackendOnePassword,
				Vault:         "Engineering",
				Item:          "jira-cli-work",
			},
			{
				Name:          "default",
				BaseURL:       "https://default.atlassian.net",
				Email:         "default@example.com",
				SecretBackend: config.SecretBackendKeyring,
			},
		},
	}

	profile := authLoginPreseedProfile(cfg, "", "default", false)
	if profile.Name != "work" {
		t.Fatalf("preseed profile = %q, want configured default profile %q", profile.Name, "work")
	}
	if profile.BaseURL != "https://work.atlassian.net" || profile.Email != "work@example.com" {
		t.Fatalf("preseed profile did not carry work metadata: %+v", profile)
	}
}

func TestAuthLoginPreseedProfileRespectsExplicitProfileName(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: []config.Profile{
			{Name: "work", BaseURL: "https://work.atlassian.net"},
			{Name: "default", BaseURL: "https://default.atlassian.net"},
		},
	}

	profile := authLoginPreseedProfile(cfg, "work", "default", true)
	if profile.Name != "default" {
		t.Fatalf("preseed profile = %q, want explicit --profile-name value", profile.Name)
	}
	if profile.BaseURL != "https://default.atlassian.net" {
		t.Fatalf("preseed profile base_url = %q, want explicit profile metadata", profile.BaseURL)
	}
}

// The review step shows what will be stored. It must surface the profile
// metadata but never the API token itself.
func TestLoginReviewSummaryShowsMetadataNotSecret(t *testing.T) {
	summary := loginReviewSummary("work", "https://company.atlassian.net", "dev@example.com", "keyring", "", "", "")
	for _, want := range []string{"work", "https://company.atlassian.net", "dev@example.com", "keyring"} {
		if !strings.Contains(summary, want) {
			t.Errorf("review summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(strings.ToLower(summary), "secret") {
		t.Errorf("review summary should not reference the secret value:\n%s", summary)
	}
}

func TestLoginReviewSummaryIncludes1PasswordCoordinates(t *testing.T) {
	summary := loginReviewSummary("work", "https://company.atlassian.net", "dev@example.com", "1password", "my.1password.com", "Engineering", "jira-cli-work")
	for _, want := range []string{"my.1password.com", "Engineering", "jira-cli-work"} {
		if !strings.Contains(summary, want) {
			t.Errorf("1password review summary missing %q:\n%s", want, summary)
		}
	}
}

// Mid-form, a 1Password backend may be selected before vault/item are filled.
// The summary must not render blank field labels with trailing whitespace.
func TestLoginReviewSummaryOmitsEmpty1PasswordLines(t *testing.T) {
	summary := loginReviewSummary("work", "https://company.atlassian.net", "dev@example.com", "1password", "", "", "")
	for _, absent := range []string{"Account:", "Vault:", "Item:"} {
		if strings.Contains(summary, absent) {
			t.Errorf("review summary rendered an empty %q label:\n%q", absent, summary)
		}
	}
}

func TestAuthLoginQuestionsUseStructuredSelectionsAndDescriptions(t *testing.T) {
	questions := authLoginQuestions()

	backend := mustAuthLoginQuestion(t, questions, "secret_backend")
	if backend.Kind != authLoginQuestionSelect {
		t.Fatalf("secret_backend kind = %q, want select", backend.Kind)
	}
	for _, want := range []string{"keyring", "1password"} {
		if !slices.ContainsFunc(backend.Options, func(option authLoginOption) bool {
			return option.Value == want && option.Label != "" && option.Description != ""
		}) {
			t.Fatalf("secret_backend options missing described value %q: %+v", want, backend.Options)
		}
	}

	for _, id := range []string{"profile_name", "base_url", "account", "credential"} {
		question := mustAuthLoginQuestion(t, questions, id)
		if question.Title == "" || question.Description == "" {
			t.Fatalf("%s missing title or description: %+v", id, question)
		}
	}
	if credential := mustAuthLoginQuestion(t, questions, "credential"); !credential.Secret {
		t.Fatalf("credential question should use hidden input: %+v", credential)
	}
}

// Jira Cloud is the only supported deployment, so the login flow must not
// offer an authentication-method choice — token auth is implied — and the
// account field must be email-focused.
func TestAuthLoginIsCloudTokenOnly(t *testing.T) {
	questions := authLoginQuestions()
	for _, question := range questions {
		if question.ID == "auth_type" {
			t.Fatalf("auth login must not offer an auth_type selection (Cloud token only): %+v", question)
		}
	}
	account := mustAuthLoginQuestion(t, questions, "account")
	if !strings.Contains(strings.ToLower(account.Title), "email") {
		t.Fatalf("account question should be email-focused for Cloud token auth: %+v", account)
	}
}

// Cloud sites are always <site>.atlassian.net, so the base-URL prompt should
// lead with the bare site name rather than demanding a full https URL.
func TestAuthLoginBaseURLPromptLeadsWithSiteName(t *testing.T) {
	q := mustAuthLoginQuestion(t, authLoginQuestions(), "base_url")
	if !strings.Contains(strings.ToLower(q.Title), "site") {
		t.Fatalf("base URL prompt should be framed as the Jira site: %+v", q)
	}
	if !strings.Contains(strings.ToLower(q.Description), "atlassian.net") {
		t.Fatalf("base URL prompt should show the site shorthand: %+v", q)
	}
}

func mustAuthLoginQuestion(t *testing.T, questions []authLoginQuestion, id string) authLoginQuestion {
	t.Helper()
	for _, question := range questions {
		if question.ID == id {
			return question
		}
	}
	t.Fatalf("missing auth login question %q in %+v", id, questions)
	return authLoginQuestion{}
}
