package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

// TestErrorEnvelopeUsesLeanAgentContract asserts a failure envelope
// carries ok:false, meta.command, meta.exit_code, data:null, and a
// structured errors[] entry.
func TestErrorEnvelopeUsesLeanAgentContract(t *testing.T) {
	env := cli.ErrorEnvelope("issue.create", errors.New("boom"))
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ok, _ := got["ok"].(bool); ok {
		t.Fatalf("failed envelope must set ok:false, got %v", got["ok"])
	}
	meta, _ := got["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("meta missing")
	}
	if meta["command"] != "issue.create" {
		t.Fatalf("meta.command = %v, want issue.create", meta["command"])
	}
	if _, has := meta["exit_code"]; !has {
		t.Fatal("error envelope missing meta.exit_code")
	}
	if got["data"] != nil {
		t.Fatalf("error envelope data must be null, got %v", got["data"])
	}
	errs, _ := got["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %v", got["errors"])
	}
}

// TestErrorEnvelopeOmitsMetaProfile asserts machine envelopes never
// carry meta.profile.
func TestErrorEnvelopeOmitsMetaProfile(t *testing.T) {
	env := cli.ErrorEnvelope("issue.view", errors.New("boom"))
	b, _ := json.Marshal(env)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	meta, _ := got["meta"].(map[string]any)
	if _, has := meta["profile"]; has {
		t.Fatalf("meta must not carry profile, got %v", meta)
	}
}

// TestStructuredErrorsCarryCodeHintRetryable asserts a typed credential
// error maps onto code/hint/retryable from the typed source, never from
// substring matching.
func TestStructuredErrorsCarryCodeHintRetryable(t *testing.T) {
	src := &config.CredentialError{
		Type:        config.ErrorTypeAuth,
		ErrCode:     config.ErrorCodeCredentialMissing,
		Message:     "no credential is stored for this profile",
		HintMsg:     "store a credential for this profile, then retry",
		IsRetryable: false,
	}
	got := cli.MapError(src)
	if got.Code != "credential_missing" {
		t.Fatalf("Code = %q, want credential_missing", got.Code)
	}
	if got.Hint != "store a credential for this profile, then retry" {
		t.Fatalf("Hint = %q", got.Hint)
	}
	if got.Retryable {
		t.Fatal("Retryable should be false")
	}
}

// TestStructuredErrorsCarryUpstreamCode asserts a provider-supplied
// upstream code is preserved verbatim and kept separate from the
// normalized code.
func TestStructuredErrorsCarryUpstreamCode(t *testing.T) {
	src := &config.CredentialError{
		Type:    config.ErrorTypeAuth,
		ErrCode: config.ErrorCodeOnePasswordUnavailable,
		Message: "1Password is unavailable",
		HintMsg: "start 1Password and retry",
		Upstream: &config.UpstreamProvider{
			Provider:       "onepassword-sdk",
			UpstreamCode:   "op-1001",
			UpstreamStatus: 6,
		},
	}
	got := cli.MapError(src)
	if got.Code != "onepassword_unavailable" {
		t.Fatalf("Code = %q", got.Code)
	}
	if got.Provider != "onepassword-sdk" || got.UpstreamCode != "op-1001" || got.UpstreamStatus != 6 {
		t.Fatalf("upstream metadata not preserved: %+v", got)
	}
}

// TestJiraAPIErrorDoesNotInventUpstreamCode asserts a Jira API error
// leaves upstream_code empty — Jira exposes no stable machine code, so
// the mapper must not synthesize one from message text.
func TestJiraAPIErrorDoesNotInventUpstreamCode(t *testing.T) {
	src := &jira.APIError{
		StatusCode:    400,
		Type:          jira.ErrorTypeValidation,
		Message:       "Field 'Projectss' does not exist or you do not have permission to view it.",
		ErrorMessages: []string{"Field 'Projectss' does not exist or you do not have permission to view it."},
	}
	got := cli.MapError(src)
	if got.UpstreamCode != "" {
		t.Fatalf("Jira API error must not carry an upstream_code, got %q", got.UpstreamCode)
	}
	if got.Code == "" {
		t.Fatal("normalized code must still be set from HTTP status")
	}
}

// TestJiraAPIErrorPreservesSchemaBackedFields asserts the schema-backed
// ErrorCollection fields (errorMessages, errors map, status) survive
// onto the envelope error as optional upstream metadata.
func TestJiraAPIErrorPreservesSchemaBackedFields(t *testing.T) {
	src := &jira.APIError{
		StatusCode:     400,
		Type:           jira.ErrorTypeValidation,
		Message:        "bad request",
		ErrorMessages:  []string{"Field 'x' is invalid."},
		FieldErrors:    map[string]string{"summary": "Summary is required."},
		UpstreamStatus: 400,
	}
	got := cli.MapError(src)
	if len(got.UpstreamMessages) != 1 || got.UpstreamMessages[0] != "Field 'x' is invalid." {
		t.Fatalf("UpstreamMessages not preserved: %+v", got.UpstreamMessages)
	}
	if got.UpstreamFieldErrors["summary"] != "Summary is required." {
		t.Fatalf("UpstreamFieldErrors not preserved: %+v", got.UpstreamFieldErrors)
	}
	if got.UpstreamStatus != 400 {
		t.Fatalf("UpstreamStatus = %d, want 400", got.UpstreamStatus)
	}
	if got.HTTPStatus != 400 {
		t.Fatalf("HTTPStatus = %d, want 400", got.HTTPStatus)
	}
}

// TestRateLimitErrorCarriesRetryAfter asserts a 429 Jira API error maps
// Retry-After onto retry_after_seconds and marks the error retryable.
func TestRateLimitErrorCarriesRetryAfter(t *testing.T) {
	src := &jira.APIError{
		StatusCode:         429,
		Type:               jira.ErrorTypeRateLimit,
		Message:            "rate limited",
		RetryAfterSeconds:  30,
		RateLimitRemaining: 2,
	}
	got := cli.MapError(src)
	if got.Type != string(cli.ErrorTypeRateLimit) {
		t.Fatalf("Type = %q, want rate_limit", got.Type)
	}
	if got.RetryAfterSeconds != 30 {
		t.Fatalf("RetryAfterSeconds = %d, want 30", got.RetryAfterSeconds)
	}
	if got.RateLimitRemaining != 2 {
		t.Fatalf("RateLimitRemaining = %d, want 2", got.RateLimitRemaining)
	}
	if !got.Retryable {
		t.Fatal("rate-limit error must be retryable")
	}
}

// TestErrorEnvelopeUsesNullData asserts data is JSON null on failure,
// not an empty object.
func TestErrorEnvelopeUsesNullData(t *testing.T) {
	env := cli.ErrorEnvelope("issue.delete", errors.New("boom"))
	b, _ := json.Marshal(env)
	if !json_contains(b, `"data":null`) {
		t.Fatalf("error envelope data must serialize as null: %s", b)
	}
}

// A context cancellation or deadline error must be mapped to a typed
// envelope entry via errors.Is — not routed through the substring
// classifier, where the wrapped url.Error text would be misbucketed.
func TestMapErrorClassifiesContextCancellation(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"deadline", fmt.Errorf("Get %q: %w", "http://x", context.DeadlineExceeded), "timeout"},
		{"canceled", fmt.Errorf("Get %q: %w", "http://x", context.Canceled), "canceled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cli.MapError(tc.err)
			if got.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", got.Code, tc.wantCode)
			}
			if !got.Retryable {
				t.Fatalf("context %s error must be retryable", tc.name)
			}
			if got.Hint == "" {
				t.Fatalf("context %s error must carry a remediation hint", tc.name)
			}
		})
	}
}

// A prompt that the user aborts or that is canceled by SIGINT/timeout
// must map to a typed envelope entry — not flow through the substring
// classifier, where "auth login aborted" would be misread as an auth
// failure (exit 1) instead of the validation-class outcome it is.
func TestMapErrorClassifiesPromptError(t *testing.T) {
	abort := cli.NewPromptError(cli.PromptAborted, "auth login", errors.New("user aborted"))
	got := cli.MapError(abort)
	if got.Type != string(cli.ErrorTypeValidation) {
		t.Fatalf("aborted prompt type = %q, want validation", got.Type)
	}
	if got.Code != "prompt_aborted" {
		t.Fatalf("aborted prompt code = %q, want prompt_aborted", got.Code)
	}
	if cli.ExitCode(got) != 3 {
		t.Fatalf("aborted prompt exit = %d, want 3", cli.ExitCode(got))
	}

	canceled := cli.NewPromptError(cli.PromptCanceled, "confirm", context.Canceled)
	gotCancel := cli.MapError(canceled)
	if gotCancel.Code != "prompt_canceled" {
		t.Fatalf("canceled prompt code = %q, want prompt_canceled", gotCancel.Code)
	}
}

func json_contains(b []byte, sub string) bool {
	return len(b) > 0 && containsString(string(b), sub)
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
