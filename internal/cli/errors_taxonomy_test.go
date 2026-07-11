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
	"issue_type_unknown":        {"validation", 3},
	"saved_query_unknown":       {"validation", 3},
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
	"validation_failed":         {"validation", 3},
	// The three credential-input codes are validation, matching both their
	// construction sites (which set a validation Type) and the semantics:
	// an empty token, an unusable profile name, and an ambiguous 1Password
	// item are "correct your input" failures, with the ambiguity code
	// paralleling user_ambiguous and board_ambiguous above. An earlier
	// revision of this table filed them under auth without a driving case;
	// these rows now record what the mapper has always emitted.
	"credential_empty":               {"validation", 3},
	"credential_namespace_collision": {"validation", 3},
	"onepassword_item_ambiguous":     {"validation", 3},
	// The env backend never stores: a write/delete against it is bad input
	// (use a different command or manage the variable), not an auth failure.
	"env_backend_read_only": {"validation", 3},
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
	// keyring_unavailable is the no-Secret-Service failure (WSL, headless
	// Linux); env_credential_unset is an env-backend profile whose
	// JIRA_TOKEN_* variable is absent. Both are credential-machinery
	// failures, so they classify with the other credential_* auth codes.
	"keyring_unavailable":  {"auth", 1},
	"env_credential_unset": {"auth", 1},
	"auth_failed":          {"auth", 1},
	// not_found (exit 2)
	"profile_not_defined": {"not_found", 2},
	"profile_incomplete":  {"not_found", 2},
	"jira_not_found":      {"not_found", 2},
	"jira_gone":           {"not_found", 2},
	"not_found":           {"not_found", 2},
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
// wantHint, when set, pins the exact per-instance hint string the mapper
// must emit; cases whose hint is the registry row's canonical string leave
// it empty (those strings are pinned by TestRegistryMatchesFrozenContract
// and the errtax hint-totality guard).
func taxonomyCases() []struct {
	name            string
	err             error
	wantCode        string
	wantHint        string
	upstreamMessage bool
} {
	return []struct {
		name            string
		err             error
		wantCode        string
		wantHint        string
		upstreamMessage bool
	}{
		{"flag unknown", NewCLIInputError(InputFlagUnknown, "unknown flag: --nope"), "flag_unknown", "Check the flag's spelling, or run the command with --help to see the ones it accepts.", true},
		{"flag foreign", &CLIInputError{Kind: InputFlagUnknown, Message: "unknown flag: --plain", Flag: "plain"}, "flag_foreign", "That flag is from a different Jira CLI — run the command with --help to see this one's flags.", true},
		{"flag value missing", NewCLIInputError(InputFlagValueMissing, "flag needs an argument: --project"), "flag_value_missing", "Give the flag a value, like --flag=value.", true},
		{"flag value invalid", NewCLIInputError(InputFlagValueInvalid, `invalid argument "x" for "--limit"`), "flag_value_invalid", "That value isn't the type the flag expects — run the command with --help to see its format.", true},
		{"flag syntax invalid", NewCLIInputError(InputFlagSyntaxInvalid, "bad flag syntax: ---x"), "flag_syntax_invalid", "Write it as --flag=value or --flag value, with nothing stray around the flag.", true},
		{"required flag missing", NewCLIInputError(InputRequiredFlagMissing, "required flag(s) --summary not set"), "required_flag_missing", "This command needs that flag — run it with --help to see which ones are required.", true},
		{"arg count invalid", NewCLIInputError(InputArgCountInvalid, "accepts 1 arg(s), received 0"), "arg_count_invalid", "Check how many arguments the command takes; its usage line is in --help.", true},
		{"arg value invalid", NewCLIInputError(InputArgValueInvalid, `invalid argument "x"`), "arg_value_invalid", "That isn't one of the accepted values — run the command with --help to see the choices.", true},
		{"issue type unknown", &jira.IssueTypeUnknownError{IssueType: "Buug", ProjectKey: "KAN", Available: []string{"Task", "Bug"}}, "issue_type_unknown", "Pass one of the project's issue types.", false},
		{"saved query unknown", NewCLIInputError(InputSavedQueryUnknown, `saved query "nope" not found`), "saved_query_unknown", "Pass one of your saved query names — they live in the queries_path directory.", true},
		{"command unknown", NewCLIInputError(InputCommandUnknown, `unknown command "nope"`), "command_unknown", "Run `jira --help` to see the available commands.", true},
		{"prompt aborted", &PromptError{Kind: PromptAborted, Prompt: "auth login"}, "prompt_aborted", "Run it again and finish the prompt, or pass the value as a flag so it doesn't need to ask.", false},
		{"prompt canceled", &PromptError{Kind: PromptCanceled, Prompt: "auth login"}, "prompt_canceled", "Run it again when you're ready to answer.", false},
		{"prompt unavailable", &PromptError{Kind: PromptUnavailable, Prompt: "auth login"}, "prompt_unavailable", "There's no terminal to prompt on — pass the value with a flag or --json-input instead.", false},
		{"markdown lossy", adf.LossyConversionError{Warning: adf.Warning{Message: "line 3: html block dropped"}}, "markdown_lossy_conversion", "", false},
		{"read only", &jira.ReadOnlyError{Method: "POST", Path: "/rest/api/3/issue"}, "read_only", "", false},
		{"dry run blocked", &jira.DryRunBlockedError{Method: "POST", Path: "/rest/api/3/issue"}, "dry_run_blocked", "", false},
		{"adf invalid", &adf.InvalidDocumentError{Got: "string", Field: "description"}, "adf_invalid", "This field takes an ADF document, not a string — see `jira agent guide adf_reference` for the shape, or use the field's *_markdown alias.", false},
		{"adf invalid wrapped", fmt.Errorf("description: %w", &adf.InvalidDocumentError{Got: "string"}), "adf_invalid", "This field takes an ADF document, not a string — see `jira agent guide adf_reference` for the shape, or use the field's *_markdown alias.", false},
		{"issue key expansion limit", &issuekey.ExpansionLimitError{Max: 100}, "issue_key_expansion_limit", "Ask for fewer keys at once, or narrow the search with a project or JQL filter.", false},
		{"user ambiguous", &jira.AmbiguousUserError{Query: "sam@example.com", Candidates: []*jira.User{{}, {}}}, "user_ambiguous", "", false},
		{"board ambiguous", boardAmbiguousStub{cands: []map[string]any{{"id": 1}, {"id": 2}}}, "board_ambiguous", "", false},
		{"board failure without candidates", boardAmbiguousStub{}, "validation_failed", "", false},
		{"credential error passthrough", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeCredentialMissing, Message: "no credential stored for profile", HintMsg: "run `jira auth login`"}, "credential_missing", "No credential is saved for this profile — run `jira auth login` to add one.", false},
		// The three cases below are constructed with Type validation, the
		// way their call sites build them; the registry records the same
		// validation classification, so driving them here pins that the
		// envelope stays validation/exit 3.
		{"credential empty", &config.CredentialError{Type: config.ErrorTypeValidation, ErrCode: config.ErrorCodeCredentialEmpty, Message: "the supplied credential is empty", HintMsg: "supply a non-empty credential"}, "credential_empty", "Provide a non-empty API token.", false},
		{"credential namespace collision", &config.CredentialError{Type: config.ErrorTypeValidation, ErrCode: config.ErrorCodeCredentialNamespaceCollision, Message: `profile name "team/dev" cannot be used as a credential key`, HintMsg: "use a profile name containing only lowercase letters, digits, hyphen, and underscore"}, "credential_namespace_collision", "Rename the profile using only lowercase letters, digits, hyphens, and underscores.", false},
		{"onepassword item ambiguous", &config.CredentialError{Type: config.ErrorTypeValidation, ErrCode: config.ErrorCodeOnePasswordItemAmbiguous, Message: `the 1Password vault has 2 items titled "jira"`, HintMsg: "give the profile a unique 1Password item title or set the item to a specific item ID"}, "onepassword_item_ambiguous", "Give the profile a unique 1Password item title, or point it at a specific item ID.", false},
		{"credential rejected at verify", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeCredentialRejected, Message: "invalid Atlassian account email or API token - Jira rejected the credential (HTTP 401 Unauthorized)", HintMsg: "check the email and that the API token is current at id.atlassian.com, or pass --skip-verify to store it without checking"}, "credential_rejected", "Check the email and API token at id.atlassian.com, or pass --skip-verify to store the credential without checking.", false},
		{"credential verify unreachable", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeCredentialVerifyUnavailable, Message: "could not verify the credential against Jira", HintMsg: "the site may be temporarily unavailable - retry, or pass --skip-verify to store it without checking"}, "credential_verify_unavailable", "Jira couldn't be reached to verify the credential — try again, or pass --skip-verify to store it without checking.", false},
		{"onepassword unsupported build", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeOnePasswordUnsupportedBuild, Message: "1Password support is unavailable in this build", HintMsg: "use a CGO-enabled source build or choose the keyring or env credential backend"}, "onepassword_unsupported_build", "This build has no 1Password support — use a source build with CGO enabled, or switch to the keyring or env backend.", false},
		// The raw D-Bus cause stays wrapped — the envelope message is the
		// typed one, so the WSL "org.freedesktop.secrets" noise never
		// classifies as validation or leaks upstream wording.
		{"keyring unavailable", config.KeyringUnavailableError(errors.New(`keyring get "default": The name org.freedesktop.secrets was not provided by any .service files`)), "keyring_unavailable", "No OS keyring is available here (common on WSL and headless Linux) — set the profile's JIRA_TOKEN_<PROFILE> variable and run `jira auth login --backend env`, or install a Secret Service such as gnome-keyring.", false},
		{"env credential unset", &config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeEnvCredentialUnset, Message: `environment variable JIRA_TOKEN_WORK is not set for profile "work"`, HintMsg: "export JIRA_TOKEN_WORK with the profile's API token"}, "env_credential_unset", "Export the profile's JIRA_TOKEN_<PROFILE> variable with the API token (JIRA_TOKEN_DEFAULT for the default profile), then retry.", false},
		{"env backend read only", &config.CredentialError{Type: config.ErrorTypeValidation, ErrCode: config.ErrorCodeEnvBackendReadOnly, Message: "the env backend cannot store a credential — JIRA_TOKEN_WORK is managed by the environment that launches jira", HintMsg: "set or unset JIRA_TOKEN_WORK in the shell or secret injector that runs jira"}, "env_backend_read_only", "The env backend only reads the profile's JIRA_TOKEN_<PROFILE> variable — set or unset it in the shell or secret manager that launches jira.", false},
		{"profile not defined", config.ProfileNotDefinedError{Name: "nope"}, "profile_not_defined", "See your profiles with `jira config profile`, or create one with `jira auth login --profile <name>`.", false},
		{"profile incomplete", config.ProfileIncompleteError{Name: "half"}, "profile_incomplete", "Finish setting up the profile — run `jira auth login --profile <name>` to give it a base URL.", false},
		{"context deadline", context.DeadlineExceeded, "timeout", "", false},
		{"context canceled", context.Canceled, "canceled", "", false},
		{"jira 400", jiraStatusError(400), "jira_bad_request", "", true},
		{"jira 401", jiraStatusError(401), "jira_unauthorized", "", true},
		{"jira 403", jiraStatusError(403), "jira_forbidden", "", true},
		{"jira 404", jiraStatusError(404), "jira_not_found", "", true},
		{"jira 409", jiraStatusError(409), "jira_conflict", "", true},
		{"jira 410", jiraStatusError(410), "jira_gone", "", true},
		{"jira 429", jiraStatusError(429), "jira_rate_limited", "", true},
		{"jira 500", jiraStatusError(500), "jira_server_error", "", true},
		// The terminal fallback no longer substring-classifies: an untyped
		// error is validation regardless of message text. These rows guard
		// against reintroducing the branches — "credential"/"not found"/
		// "rate limit"/"server" text must NOT reroute an untyped error into
		// auth/not_found/rate_limit/server. Anything that needs those codes
		// is typed at its source.
		{"untyped 'credential' text stays validation", errors.New("credential rejected by backend"), "validation_failed", "", false},
		{"untyped 'not found' text stays validation", errors.New("widget not found"), "validation_failed", "", false},
		{"untyped 'rate limit' text stays validation", errors.New("rate limit exceeded upstream"), "validation_failed", "", false},
		{"untyped 'server' text stays validation", errors.New("server exploded"), "validation_failed", "", false},
		{"untyped fallback", errors.New("something odd happened"), "validation_failed", "", false},
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
			if tc.wantHint != "" && mapped.Hint != tc.wantHint {
				t.Errorf("hint = %q, want %q", mapped.Hint, tc.wantHint)
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
		// HintMsg; the mapper passes them through. The validation-typed
		// credential codes are driven above; these remaining auth-typed
		// ones ride the passthrough case.
		"credential_source_conflict":     true,
		"credential_backend_unavailable": true,
		"credential_migration_failed":    true,
		"credential_cleanup_failed":      true,
		"onepassword_unavailable":        true,
		// The type-default catch-alls. The legacy substring classifier was
		// retired — classifyUntyped now always returns validation_failed —
		// so these are produced only at call sites, never by the mapper:
		// not_found by cli.NotFoundError, and the auth/rate_limit/server
		// defaults by errtax.DefaultCode where the Jira client or a
		// multi-key aggregate classifies a status/type that has no more
		// specific code.
		"not_found":    true,
		"auth_failed":  true,
		"rate_limited": true,
		"server_error": true,
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

// TestUnknownIssueTypeMapsToValidationWithSuggestions pins the reclassification
// of a bad --type: the CLI resolves the name against the fetched issuetypes
// list in-code, so a miss is a validation failure (exit 3) — not the Jira 404
// it used to synthesize — and the project's valid type names ride the
// suggestions field, never the static hint.
func TestUnknownIssueTypeMapsToValidationWithSuggestions(t *testing.T) {
	err := &jira.IssueTypeUnknownError{IssueType: "Buug", ProjectKey: "KAN", Available: []string{"Task", "Bug", "Story"}}
	// Wrapped exactly as the mutation pipeline surfaces it, to prove the
	// classification survives the join + %w chain.
	wrapped := fmt.Errorf("screen schema unavailable: project or issue type not found: %w", err)
	got := MapError(wrapped)

	if got.Type != string(ErrorTypeValidation) {
		t.Errorf("Type = %q, want validation", got.Type)
	}
	if got.Code != "issue_type_unknown" {
		t.Errorf("Code = %q, want issue_type_unknown", got.Code)
	}
	if ExitCode(got) != 3 {
		t.Errorf("ExitCode = %d, want 3", ExitCode(got))
	}
	if got.Flag != "type" {
		t.Errorf("Flag = %q, want type", got.Flag)
	}
	want := []string{"Task", "Bug", "Story"}
	if len(got.Suggestions) != len(want) {
		t.Fatalf("Suggestions = %v, want %v", got.Suggestions, want)
	}
	for i := range want {
		if got.Suggestions[i] != want[i] {
			t.Fatalf("Suggestions = %v, want %v", got.Suggestions, want)
		}
	}
	if !strings.Contains(got.Message, "KAN") || !strings.Contains(got.Message, "Buug") {
		t.Errorf("Message = %q, want it to name the bad type and project", got.Message)
	}
	if strings.Contains(got.Hint, "Task") {
		t.Errorf("valid types must ride suggestions, not the static hint: %q", got.Hint)
	}
}

// TestRegistryMatchesFrozenContract restores the two-source lock after the
// registry's promotion into production: the frozen, human-reviewed table
// above is the contract, errtax is the implementation, and the two must
// agree code-for-code on type and exit. Without this, driving MapError
// against registry-derived expectations would assert the registry against
// itself.
func TestRegistryMatchesFrozenContract(t *testing.T) {
	t.Parallel()
	codes := errtax.Codes()
	if len(codes) != len(taxonomyRegistry) {
		t.Errorf("registry has %d codes, frozen contract has %d — the two key sets must be identical", len(codes), len(taxonomyRegistry))
	}
	for code := range taxonomyRegistry {
		if _, ok := errtax.Lookup(errtax.Code(code)); !ok {
			t.Errorf("frozen contract code %q is not registered", code)
		}
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			want, ok := taxonomyRegistry[string(code)]
			if !ok {
				t.Fatalf("registry code %q is missing from the frozen contract", code)
			}
			spec, _ := errtax.Lookup(code)
			if string(spec.Type) != want.errType {
				t.Errorf("type = %q, want %q", spec.Type, want.errType)
			}
			if spec.Exit != want.exit {
				t.Errorf("exit = %d, want %d", spec.Exit, want.exit)
			}
		})
	}
}

// TestClassifyStatusRegistryLockstep holds jira.ClassifyStatus and
// jira.CodeForStatus to the same classification for a hand-written status
// list. The check is meaningful only because ClassifyStatus stays an
// independent status→type switch — deriving it from CodeForStatus plus the
// registry would collapse both sides into one source.
func TestClassifyStatusRegistryLockstep(t *testing.T) {
	t.Parallel()
	statuses := []int{400, 401, 403, 404, 409, 410, 413, 418, 429, 500, 502, 503}
	for _, status := range statuses {
		code := jira.CodeForStatus(status)
		spec, ok := errtax.Lookup(code)
		if !ok {
			t.Errorf("status %d resolves to unregistered code %q", status, code)
			continue
		}
		if got := jira.ClassifyStatus(status); got != spec.Type {
			t.Errorf("status %d: ClassifyStatus = %q, registry says %q for code %q", status, got, spec.Type, code)
		}
	}
}

// TestWrappedCancellationKeepsContextIdentity pins the mapper's tier
// order: a typed error whose Unwrap chain carries a context cancellation
// classifies as canceled/timeout (the context tier precedes the Coded
// tier), while a canceled prompt keeps its prompt identity (the prompt
// tier precedes the context tier). Neither case was tested before the
// registry promotion — CI was blind to the ordering.
func TestWrappedCancellationKeepsContextIdentity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantExit int
	}{
		{
			"api error wrapping canceled",
			&jira.APIError{StatusCode: 500, Type: jira.ErrorTypeServer, Message: "read response body: context canceled", Cause: context.Canceled},
			"canceled", 6,
		},
		{
			"credential error wrapping deadline",
			&config.CredentialError{Type: config.ErrorTypeAuth, ErrCode: config.ErrorCodeCredentialBackendUnavailable, Message: "the keyring credential backend is unavailable", HintMsg: "check that the credential backend is running and reachable, then retry", Wrapped: context.DeadlineExceeded},
			"timeout", 7,
		},
		{
			"prompt error wrapping canceled",
			&PromptError{Kind: PromptCanceled, Prompt: "auth login", Err: context.Canceled},
			"prompt_canceled", 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mapped := MapError(tc.err)
			if mapped.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q (mapped: %+v)", mapped.Code, tc.wantCode, mapped)
			}
			if got := ExitCode(mapped); got != tc.wantExit {
				t.Errorf("exit = %d, want %d", got, tc.wantExit)
			}
		})
	}
}
