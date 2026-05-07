package main

import (
	"slices"
	"strings"
	"testing"
)

func TestAuthLoginQuestionsUseStructuredSelectionsAndDescriptions(t *testing.T) {
	questions := authLoginQuestions()

	authType := mustAuthLoginQuestion(t, questions, "auth_type")
	if authType.Kind != authLoginQuestionSelect {
		t.Fatalf("auth_type kind = %q, want select", authType.Kind)
	}
	for _, want := range []string{"token", "basic", "pat", "mtls"} {
		if !slices.ContainsFunc(authType.Options, func(option authLoginOption) bool {
			return option.Value == want && option.Label != "" && option.Description != ""
		}) {
			t.Fatalf("auth_type options missing described value %q: %+v", want, authType.Options)
		}
	}
	if slices.ContainsFunc(authType.Options, func(option authLoginOption) bool {
		return option.Value == "oauth2"
	}) {
		t.Fatalf("auth login should not offer oauth2 until an OAuth flow is implemented: %+v", authType.Options)
	}

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

func TestUnsupportedAuthTypesDoNotRequireCredentialStorage(t *testing.T) {
	if err := validateAuthLoginType("oauth2"); err == nil {
		t.Fatalf("oauth2 login should be unsupported")
	}
	if authLoginNeedsCredential("oauth2") {
		t.Fatalf("oauth2 login should not ask for a raw credential")
	}
	for _, authType := range []string{"token", "basic", "pat"} {
		if !authLoginNeedsCredential(authType) {
			t.Fatalf("%s login should ask for credential storage", authType)
		}
	}
	if err := validateAuthLoginType("mtls"); err != nil {
		t.Fatalf("mtls login should be supported as metadata: %v", err)
	}
	if authLoginNeedsCredential("mtls") {
		t.Fatalf("mtls login should not ask for an API token/password credential")
	}

	credential := mustAuthLoginQuestion(t, authLoginQuestions(), "credential")
	if slices.ContainsFunc(credential.Options, func(option authLoginOption) bool {
		return option.Value == "oauth2"
	}) {
		t.Fatalf("credential question should not be tied to oauth2 options: %+v", credential.Options)
	}
	if strings.Contains(credential.Description, "OAuth refresh token") {
		t.Fatalf("credential description should not suggest OAuth raw-token entry: %q", credential.Description)
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
