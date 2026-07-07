package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/errtax"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
)

// taxonomyRegistry is the locked KAN-270 contract table: every code the CLI
// may emit, with its type and exit code. The guards below hold the mapper to
// it — an ad-hoc code, an empty hint, or a type/exit disagreement fails here
// before it can reach an envelope.
var taxonomyRegistry = map[string]struct {
	errType string
	exit    int
}{
	// validation (exit 3)
	"flag_unknown":              {"validation", 3},
	"flag_foreign":              {"validation", 3},
	"flag_value_missing":        {"validation", 3},
	"flag_value_invalid":        {"validation", 3},
	"flag_syntax_invalid":       {"validation", 3},
	"required_flag_missing":     {"validation", 3},
	"arg_count_invalid":         {"validation", 3},
	"arg_value_invalid":         {"validation", 3},
	"command_unknown":           {"validation", 3},
	"prompt_aborted":            {"validation", 3},
	"prompt_canceled":           {"validation", 3},
	"prompt_unavailable":        {"validation", 3},
	"markdown_lossy_conversion": {"validation", 3},
	"read_only":                 {"validation", 3},
	"adf_invalid":               {"validation", 3},
	"issue_key_expansion_limit": {"validation", 3},
	"user_ambiguous":            {"validation", 3},
	"board_ambiguous":           {"validation", 3},
	"dry_run_blocked":           {"validation", 3},
	// The three credential-input codes are validation, matching both their
	// construction sites (which set a validation Type) and the semantics:
	// an empty token, an unusable profile name, and an ambiguous 1Password
	// item are "correct your input" failures, with the ambiguity code
	// paralleling user_ambiguous and board_ambiguous above.
	"credential_empty":               {"validation", 3},
	"credential_namespace_collision": {"validation", 3},
	"onepassword_item_ambiguous":     {"validation", 3},
	"validation_failed":              {"validation", 3},
	// auth (exit 1)
	"jira_unauthorized":              {"auth", 1},
	"jira_forbidden":                 {"auth", 1},
	"credential_missing":             {"auth", 1},
	"credential_source_conflict":     {"auth", 1},
	"credential_backend_unavailable": {"auth", 1},
	"credential_migration_failed":    {"auth", 1},
	"credential_cleanup_failed":      {"auth", 1},
	"onepassword_unavailable":        {"auth", 1},
	"credential_rejected":            {"auth", 1},
	"credential_verify_unavailable":  {"auth", 1},
	"onepassword_unsupported_build":  {"auth", 1},
	"auth_failed":                    {"auth", 1},
	// not_found (exit 2)
	"profile_not_defined": {"not_found", 2},
	"profile_incomplete":  {"not_found", 2},
	"jira_not_found":    {"not_found", 2},
	"jira_gone":         {"not_found", 2},
	"not_found":         {"not_found", 2},
	// rate_limit (exit 4)
	"jira_rate_limited": {"rate_limit", 4},
	"rate_limited":      {"rate_limit", 4},
	// 400 and 409 deliberately classify as validation (exit 3): both are
	// client-actionable — correct the input, or re-fetch and retry — where
	// exit 5 would read as an upstream fault. 410 lives with not_found above.
	"jira_bad_request": {"validation", 3},
	"jira_conflict":    {"validation", 3},
	// server (exit 5, plus canceled=6 / timeout=7)
	"jira_server_error": {"server", 5},
	"server_error":      {"server", 5},
	"canceled":          {"server", 6},
	"timeout":           {"server", 7},
}

// boardAmbiguousStub mirrors boardscope.ValidationError: a Coded
// validation failure whose code varies with the candidate rows.
type boardAmbiguousStub struct{ cands []map[string]any }

func (e boardAmbiguousStub) Error() string { return `board "dev" matched 2 boards` }

func (e boardAmbiguousStub) Code() errtax.Code {
	if len(e.cands) > 0 {
		return errtax.CodeBoardAmbiguous
	}
	return errtax.CodeValidationFailed
}

func (e boardAmbiguousStub) CandidateRows() []map[string]any { return e.cands }

func jiraStatusError(status int) error {
	return &jira.APIError{
		StatusCode: status,
		Type:       jira.ClassifyStatus(status),
		Message:    "upstream text",
	}
}

// taxonomyCases drives MapError once per emittable code. upstreamMessage
// marks cases whose message text originates outside the repo (Jira body
// text, pflag output we merely relay) — the style guard skips those.
func taxonomyCases() []struct {
	name            string
	err             error
	wantCode        string
	upstreamMessage bool
} {
	return []struct {
		name            string
		err             error
		wantCode        string
		upstreamMessage bool
	}{
		{"flag unknown", NewCLIInputError(InputFlagUnknown, "unknown flag: --nope"), "flag_unknown", true},
		{"flag value missing", NewCLIInputError(InputFlagValueMissing, "flag needs an argument: --project"), "flag_value_missing", true},
		{"flag value invalid", NewCLIInputError(InputFlagValueInvalid, `invalid argument "x" for "--limit"`), "flag_value_invalid", true},
		{"flag syntax invalid", NewCLIInputError(InputFlagSyntaxInvalid, "bad flag syntax: ---x"), "flag_syntax_invalid", true},
		{"required flag missing", NewCLIInputError(InputRequiredFlagMissing, "required flag(s) --summary not set"), "required_flag_missing", true},
		{"arg count invalid", NewCLIInputError(InputArgCountInvalid, "accepts 1 arg(s), received 0"), "arg_count_invalid", true},
		{"arg value invalid", NewCLIInputError(InputArgValueInvalid, `invalid argument "x"`), "arg_value_invalid", true},
		{"command unknown", NewCLIInputError(InputCommandUnknown, `unknown command "nope"`), "command_unknown", true},
		{"prompt aborted", &PromptError{Kind: PromptAborted, Prompt: "auth login"}, "prompt_aborted", false},
		{"prompt canceled", &PromptError{Kind: PromptCanceled, Prompt: "auth login"}, "prompt_canceled", false},
		{"prompt unavailable", &PromptError{Kind: PromptUnavailable, Prompt: "auth login"}, "prompt_unavailable", false},
		{"markdown lossy", adf.LossyConversionError{Warning: adf.Warning{Message: "line 3: html block dropped"}}, "markdown_lossy_conversion", false},
		{"read only", &jira.ReadOnlyError{Method: "POST", Path: "/rest/api/3/issue"}, "read_only", false},
		{"dry run blocked", &jira.DryRunBlockedError{Method: "POST", Path: "/rest/api/3/issue"}, "dry_run_blocked", false},
		{"adf invalid", &adf.InvalidDocumentError{Got: "string", Field: "description"}, "adf_invalid", false},
		{"adf invalid wrapped", fmt.Errorf("description: %w", &adf.InvalidDocumentError{Got: "string"}), "adf_invalid", false},
		{"issue key expansion limit", &issuekey.ExpansionLimitError{Max: 100}, "issue_key_expansion_limit", false},
		{"user ambiguous", &jira.AmbiguousUserError{Query: "sam@example.com", Candidates: []*jira.User{{}, {}}}, "user_ambiguous", false},
		{"board ambiguous", boardAmbiguousStub{cands: []map[string]any{{"id": 1}, {"id": 2}}}, "board_ambiguous", false},
		{"board failure without candidates", boardAmbiguousStub{}, "validation_failed", false},
		{"credential error passthrough", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeCredentialMissing, Message: "no credential stored for profile", HintMsg: "run `jira auth login`"}, "credential_missing", false},
		{"profile not defined", config.ProfileNotDefinedError{Name: "nope"}, "profile_not_defined", false},
		{"profile incomplete", config.ProfileIncompleteError{Name: "half"}, "profile_incomplete", false},
		{"context deadline", context.DeadlineExceeded, "timeout", false},
		{"context canceled", context.Canceled, "canceled", false},
		{"jira 400", jiraStatusError(400), "jira_bad_request", true},
		{"jira 401", jiraStatusError(401), "jira_unauthorized", true},
		{"jira 403", jiraStatusError(403), "jira_forbidden", true},
		{"jira 404", jiraStatusError(404), "jira_not_found", true},
		{"jira 409", jiraStatusError(409), "jira_conflict", true},
		{"jira 410", jiraStatusError(410), "jira_gone", true},
		{"jira 429", jiraStatusError(429), "jira_rate_limited", true},
		{"jira 500", jiraStatusError(500), "jira_server_error", true},
		{"untyped auth", errors.New("credential rejected by backend"), "auth_failed", false},
		{"untyped not found", errors.New("widget not found"), "not_found", false},
		{"untyped rate limit", errors.New("rate limit exceeded upstream"), "rate_limited", false},
		{"untyped server", errors.New("server exploded"), "server_error", false},
		{"untyped fallback", errors.New("something odd happened"), "validation_failed", false},
	}
}

// TestErrorTaxonomyClosedHintedAndExitAligned is the core KAN-270 guard:
// every driveable code maps into the locked registry (closed set), carries a
// non-empty hint (totality), and agrees with the registry on type and exit
// code. A new ad-hoc code, an empty hint, or a type/exit drift fails here.
func TestErrorTaxonomyClosedHintedAndExitAligned(t *testing.T) {
	for _, tc := range taxonomyCases() {
		t.Run(tc.name, func(t *testing.T) {
			mapped := MapError(tc.err)
			if mapped.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q (mapped: %+v)", mapped.Code, tc.wantCode, mapped)
			}
			reg, known := taxonomyRegistry[mapped.Code]
			if !known {
				t.Fatalf("code %q is not in the documented registry — add it to the contract or fix the mapper", mapped.Code)
			}
			if mapped.Type != reg.errType {
				t.Errorf("type = %q, want %q for code %q", mapped.Type, reg.errType, mapped.Code)
			}
			if got := ExitCode(mapped); got != reg.exit {
				t.Errorf("exit = %d, want %d for code %q", got, reg.exit, mapped.Code)
			}
			if mapped.Hint == "" {
				t.Errorf("code %q ships an empty hint — every code carries a next step", mapped.Code)
			}
		})
	}
}

// TestErrorTaxonomyRegistryFullyDriven keeps the guard honest in the other
// direction: every registry code is either driven by a case above or is on
// the explicit call-site-constructed list (credential codes whose values are
// supplied where the error is built, not by the mapper).
func TestErrorTaxonomyRegistryFullyDriven(t *testing.T) {
	callSiteOnly := map[string]bool{
		// CredentialError codes are set at construction sites with their
		// HintMsg; the mapper passes them through (covered by the
		// credential passthrough case). One representative is driven.
		"credential_empty":               true,
		"credential_source_conflict":     true,
		"credential_backend_unavailable": true,
		"credential_migration_failed":    true,
		"credential_cleanup_failed":      true,
		"credential_namespace_collision": true,
		"onepassword_item_ambiguous":     true,
		"onepassword_unavailable":        true,
		// Set at their construction sites too; driven cases arrive with
		// the registry-driven guard.
		"flag_foreign":                  true,
		"credential_rejected":           true,
		"credential_verify_unavailable": true,
		"onepassword_unsupported_build": true,
	}
	driven := map[string]bool{}
	for _, tc := range taxonomyCases() {
		driven[tc.wantCode] = true
	}
	for code := range taxonomyRegistry {
		if !driven[code] && !callSiteOnly[code] {
			t.Errorf("registry code %q has no driving case — add one or move it to the call-site list", code)
		}
	}
	for code := range callSiteOnly {
		if _, ok := taxonomyRegistry[code]; !ok {
			t.Errorf("call-site code %q is not in the registry", code)
		}
	}
}

// TestErrorMessagesFollowStyle enforces golang-error-handling #3 on every
// message the repo authors: lowercase start, no trailing punctuation. Text
// merely relayed from outside (Jira bodies, pflag output) is exempt, as is
// the hint — that is the user-facing translation, full sentences are fine.
func TestErrorMessagesFollowStyle(t *testing.T) {
	for _, tc := range taxonomyCases() {
		if tc.upstreamMessage {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			mapped := MapError(tc.err)
			if mapped.Message == "" {
				t.Fatal("empty message")
			}
			first := []rune(mapped.Message)[0]
			if unicode.IsUpper(first) {
				t.Errorf("message starts uppercase: %q", mapped.Message)
			}
			if strings.ContainsAny(mapped.Message[len(mapped.Message)-1:], ".!?") {
				t.Errorf("message ends with punctuation: %q", mapped.Message)
			}
		})
	}
}

// TestADFStringLeakTranslated pins the behavioral fix: a payload that
// supplies a plain string where an ADF document belongs yields the stable
// adf_invalid code with the clean message — the raw json unmarshal error
// text never reaches the envelope.
func TestADFStringLeakTranslated(t *testing.T) {
	_, _, perr := adf.Parse([]byte(`"just some text"`))
	if perr == nil {
		t.Fatal("string parsed as an ADF document")
	}
	mapped := MapError(fmt.Errorf("description: %w", perr))
	if mapped.Code != "adf_invalid" {
		t.Fatalf("code = %q, want adf_invalid", mapped.Code)
	}
	if strings.Contains(mapped.Message, "cannot unmarshal") || strings.Contains(mapped.Message, "json:") {
		t.Fatalf("raw json error leaked into the message: %q", mapped.Message)
	}
	if !strings.Contains(mapped.Hint, "ADF document") {
		t.Errorf("hint does not explain the required shape: %q", mapped.Hint)
	}
}

// TestOutputFlagValueRoutesThroughFlagValueInvalid pins flag-value
// consistency for the one flag validated outside pflag: a bad --output
// value carries the same flag_value_invalid code and names the flag.
func TestOutputFlagValueRoutesThroughFlagValueInvalid(t *testing.T) {
	_, err := ParseOutputMode("banana")
	if err == nil {
		t.Fatal("banana accepted as an output mode")
	}
	mapped := MapError(err)
	if mapped.Code != "flag_value_invalid" {
		t.Fatalf("code = %q, want flag_value_invalid", mapped.Code)
	}
	if mapped.Flag != "output" {
		t.Errorf("flag = %q, want output", mapped.Flag)
	}
	if ExitCode(mapped) != 3 {
		t.Errorf("exit = %d, want 3", ExitCode(mapped))
	}
}
