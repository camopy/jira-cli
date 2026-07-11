package errtax

import (
	"maps"
	"slices"
)

// Type is the coarse error classification the envelope's `type` field
// carries. Each type owns one default process exit code (see [ExitFor]);
// individual codes may pin their own exit in the registry.
type Type string

// The five envelope error types, plus the named zero value.
const (
	// TypeUnknown is the zero value — never emitted; it marks an
	// unclassified error before the registry resolves it.
	TypeUnknown    Type = ""
	TypeAuth       Type = "auth"
	TypeNotFound   Type = "not_found"
	TypeValidation Type = "validation"
	TypeRateLimit  Type = "rate_limit"
	TypeServer     Type = "server"
)

// Code is the stable snake_case error code the envelope's `code` field
// carries. Agents branch on Code, never on message text; every code has a
// registry row.
type Code string

// CodeUnknown is the zero value — [Lookup] misses on it, so an
// unclassified error fails closed to the server type and generic hint.
const CodeUnknown Code = ""

// Validation codes (exit 3): command-line input, prompts, payload shape,
// and the transport-layer mutation guards.
const (
	CodeFlagUnknown Code = "flag_unknown"
	// CodeFlagForeign is a recognized flag from a different Jira CLI — not
	// a typo, so it gets an orientation hint instead of a spelling one;
	// this CLI's equivalents ride the envelope's suggestions field.
	CodeFlagForeign         Code = "flag_foreign"
	CodeFlagValueMissing    Code = "flag_value_missing"
	CodeFlagValueInvalid    Code = "flag_value_invalid"
	CodeFlagSyntaxInvalid   Code = "flag_syntax_invalid"
	CodeRequiredFlagMissing Code = "required_flag_missing"
	CodeArgCountInvalid     Code = "arg_count_invalid"
	CodeArgValueInvalid     Code = "arg_value_invalid"
	// CodeIssueTypeUnknown is a --type value that names no issue type on the
	// project's create screen. The CLI resolves the type against the fetched
	// list in-code, so a miss is a bad input value (validation, exit 3), not a
	// Jira 404 — the valid names ride the envelope's suggestions field.
	CodeIssueTypeUnknown Code = "issue_type_unknown"
	// CodeSavedQueryUnknown is a `search saved NAME` value that matches no
	// saved query. The names live in the user's queries directory, not in
	// --help, so this gets its own hint rather than reusing arg_value_invalid.
	CodeSavedQueryUnknown       Code = "saved_query_unknown"
	CodeCommandUnknown          Code = "command_unknown"
	CodePromptAborted           Code = "prompt_aborted"
	CodePromptCanceled          Code = "prompt_canceled"
	CodePromptUnavailable       Code = "prompt_unavailable"
	CodeMarkdownLossyConversion Code = "markdown_lossy_conversion"
	CodeReadOnly                Code = "read_only"
	CodeADFInvalid              Code = "adf_invalid"
	CodeIssueKeyExpansionLimit  Code = "issue_key_expansion_limit"
	CodeUserAmbiguous           Code = "user_ambiguous"
	CodeBoardAmbiguous          Code = "board_ambiguous"
	CodeDryRunBlocked           Code = "dry_run_blocked"
	// CodeCredentialEmpty, CodeCredentialNamespaceCollision, and
	// CodeOnePasswordItemAmbiguous classify as validation, matching their
	// construction sites: an empty token, an unusable profile name, and an
	// ambiguous 1Password item are input problems — the fix is "correct
	// your input", not "re-authenticate". The ambiguity code is the
	// credential-side parallel of user_ambiguous and board_ambiguous.
	CodeCredentialEmpty              Code = "credential_empty"
	CodeCredentialNamespaceCollision Code = "credential_namespace_collision"
	CodeOnePasswordItemAmbiguous     Code = "onepassword_item_ambiguous"
	// CodeEnvBackendReadOnly: a write or delete was attempted against the env
	// secret backend, which only ever reads its JIRA_TOKEN_* variable.
	CodeEnvBackendReadOnly Code = "env_backend_read_only"
	// CodeValidationFailed is the validation catch-all; every specific
	// validation code above must pre-empt it.
	CodeValidationFailed Code = "validation_failed"
	// CodeJiraBadRequest (400) and CodeJiraConflict (409) classify as
	// validation deliberately: both are client-actionable — correct the
	// input, or re-fetch and retry — where a server type would read as an
	// upstream fault.
	CodeJiraBadRequest Code = "jira_bad_request"
	CodeJiraConflict   Code = "jira_conflict"
)

// Auth codes (exit 1): rejected or unusable credentials, whether Jira said
// so (jira_*) or the local credential machinery did (credential_*).
const (
	CodeJiraUnauthorized             Code = "jira_unauthorized"
	CodeJiraForbidden                Code = "jira_forbidden"
	CodeCredentialMissing            Code = "credential_missing"
	CodeCredentialSourceConflict     Code = "credential_source_conflict"
	CodeCredentialBackendUnavailable Code = "credential_backend_unavailable"
	CodeCredentialMigrationFailed    Code = "credential_migration_failed"
	CodeCredentialCleanupFailed      Code = "credential_cleanup_failed"
	CodeOnePasswordUnavailable       Code = "onepassword_unavailable"
	// CodeCredentialRejected is a login-verification rejection: Jira said
	// the email/token pair is wrong.
	CodeCredentialRejected Code = "credential_rejected"
	// CodeCredentialVerifyUnavailable: Jira could not be reached to verify
	// a credential at login — distinct from the credential store being
	// unreachable (credential_backend_unavailable).
	CodeCredentialVerifyUnavailable Code = "credential_verify_unavailable"
	// CodeOnePasswordUnsupportedBuild: the binary was built without CGO,
	// so 1Password support is compiled out entirely.
	CodeOnePasswordUnsupportedBuild Code = "onepassword_unsupported_build"
	// CodeKeyringUnavailable: the OS keyring cannot service requests at all —
	// no Secret Service on the D-Bus (WSL, headless Linux), an unsupported
	// platform — distinct from credential_backend_unavailable, whose hint
	// assumes the backend exists and merely hiccuped.
	CodeKeyringUnavailable Code = "keyring_unavailable"
	// CodeEnvCredentialUnset: a profile using the env secret backend has no
	// JIRA_TOKEN_* variable set — the credential's single source is absent.
	CodeEnvCredentialUnset Code = "env_credential_unset"
	// CodeAuthFailed is the auth catch-all for untyped credential failures.
	CodeAuthFailed Code = "auth_failed"
)

// Not-found codes (exit 2).
const (
	// CodeProfileNotDefined and CodeProfileIncomplete split the profile
	// failure by remediation — create the profile vs finish setting it
	// up. Both classify as not_found so a bad --profile fails closed.
	CodeProfileNotDefined Code = "profile_not_defined"
	CodeProfileIncomplete Code = "profile_incomplete"
	CodeJiraNotFound      Code = "jira_not_found"
	// CodeJiraGone (410) is a permanently deleted resource: not-found tells
	// an agent to fix the reference, where validation would suggest
	// correcting the request.
	CodeJiraGone Code = "jira_gone"
	// CodeNotFound is the not-found catch-all for untyped failures.
	CodeNotFound Code = "not_found"
)

// Rate-limit codes (exit 4).
const (
	CodeJiraRateLimited Code = "jira_rate_limited"
	// CodeRateLimited is the rate-limit catch-all for untyped failures.
	CodeRateLimited Code = "rate_limited"
)

// Server codes (exit 5), plus the two lifecycle codes that pin their own
// exits: canceled (6) and timeout (7) so a caller can distinguish an
// interrupted or expired invocation from a real backend failure.
const (
	CodeJiraServerError Code = "jira_server_error"
	// CodeServerError is the server catch-all for untyped failures.
	CodeServerError Code = "server_error"
	CodeCanceled    Code = "canceled"
	CodeTimeout     Code = "timeout"
)

// Spec is one registry row: everything the envelope derives from a code.
type Spec struct {
	// Type is the coarse classification the envelope's `type` field carries.
	Type Type
	// Exit is the process exit code for this code — usually the type's
	// default, pinned per code so canceled (6) and timeout (7) can diverge
	// from their server type.
	Exit int
	// Hint is the next-action remediation, in the plain words a person
	// would use. Never empty, and always static: a runtime specific
	// belongs in the error message or a structured envelope field.
	Hint string
	// Retryable reports whether retrying the same invocation could succeed.
	Retryable bool
}

// genericHint is the fail-closed remediation when no registry row applies.
const genericHint = "Rerun with --debug and report the failure if it persists."

// registry binds every emittable code to its classification. Populated
// once by this literal and never mutated; access it through [Lookup],
// [HintFor], and [Codes]. When adding a code, write its Hint per the hint
// style in CONTRIBUTING.md.
var registry = map[Code]Spec{
	// validation (exit 3)
	CodeFlagUnknown:             {Type: TypeValidation, Exit: 3, Hint: "Check the flag's spelling, or run the command with --help to see the ones it accepts.", Retryable: false},
	CodeFlagForeign:             {Type: TypeValidation, Exit: 3, Hint: "That flag is from a different Jira CLI — run the command with --help to see this one's flags.", Retryable: false},
	CodeFlagValueMissing:        {Type: TypeValidation, Exit: 3, Hint: "Give the flag a value, like --flag=value.", Retryable: false},
	CodeFlagValueInvalid:        {Type: TypeValidation, Exit: 3, Hint: "That value isn't the type the flag expects — run the command with --help to see its format.", Retryable: false},
	CodeFlagSyntaxInvalid:       {Type: TypeValidation, Exit: 3, Hint: "Write it as --flag=value or --flag value, with nothing stray around the flag.", Retryable: false},
	CodeRequiredFlagMissing:     {Type: TypeValidation, Exit: 3, Hint: "This command needs that flag — run it with --help to see which ones are required.", Retryable: false},
	CodeArgCountInvalid:         {Type: TypeValidation, Exit: 3, Hint: "Check how many arguments the command takes; its usage line is in --help.", Retryable: false},
	CodeArgValueInvalid:         {Type: TypeValidation, Exit: 3, Hint: "That isn't one of the accepted values — run the command with --help to see the choices.", Retryable: false},
	CodeIssueTypeUnknown:        {Type: TypeValidation, Exit: 3, Hint: "Pass one of the project's issue types.", Retryable: false},
	CodeSavedQueryUnknown:       {Type: TypeValidation, Exit: 3, Hint: "Pass one of your saved query names — they live in the queries_path directory.", Retryable: false},
	CodeCommandUnknown:          {Type: TypeValidation, Exit: 3, Hint: "Run `jira --help` to see the available commands.", Retryable: false},
	CodePromptAborted:           {Type: TypeValidation, Exit: 3, Hint: "Run it again and finish the prompt, or pass the value as a flag so it doesn't need to ask.", Retryable: false},
	CodePromptCanceled:          {Type: TypeValidation, Exit: 3, Hint: "Run it again when you're ready to answer.", Retryable: false},
	CodePromptUnavailable:       {Type: TypeValidation, Exit: 3, Hint: "There's no terminal to prompt on — pass the value with a flag or --json-input instead.", Retryable: false},
	CodeMarkdownLossyConversion: {Type: TypeValidation, Exit: 3, Hint: "Rewrite the flagged Markdown, or add --adf-best-effort to accept the downgrade.", Retryable: false},
	CodeReadOnly:                {Type: TypeValidation, Exit: 3, Hint: "Unset JIRA_READ_ONLY, set the profile's read_only=false, or switch to a profile that allows writes.", Retryable: false},
	CodeADFInvalid:              {Type: TypeValidation, Exit: 3, Hint: "This field takes an ADF document, not a string — see `jira agent guide adf_reference` for the shape, or use the field's *_markdown alias.", Retryable: false},
	CodeIssueKeyExpansionLimit:  {Type: TypeValidation, Exit: 3, Hint: "Ask for fewer keys at once, or narrow the search with a project or JQL filter.", Retryable: false},
	CodeUserAmbiguous:           {Type: TypeValidation, Exit: 3, Hint: "Run `jira user search <name>` to see each match's account ID, then pass that ID instead of the email.", Retryable: false},
	CodeBoardAmbiguous:          {Type: TypeValidation, Exit: 3, Hint: "Run `jira boards list` to see the boards and their IDs, then pass one with --board — or set a default board for the profile.", Retryable: false},
	CodeDryRunBlocked:           {Type: TypeValidation, Exit: 3, Hint: "This was a dry run — run it again without --dry-run to actually make the change.", Retryable: false},
	// The three credential-input codes classify as validation (see the
	// const block): correct the input, don't re-authenticate.
	CodeCredentialEmpty:              {Type: TypeValidation, Exit: 3, Hint: "Provide a non-empty API token.", Retryable: false},
	CodeCredentialNamespaceCollision: {Type: TypeValidation, Exit: 3, Hint: "Rename the profile using only lowercase letters, digits, hyphens, and underscores.", Retryable: false},
	CodeOnePasswordItemAmbiguous:     {Type: TypeValidation, Exit: 3, Hint: "Give the profile a unique 1Password item title, or point it at a specific item ID.", Retryable: false},
	CodeEnvBackendReadOnly:           {Type: TypeValidation, Exit: 3, Hint: "The env backend only reads the profile's JIRA_TOKEN_<PROFILE> variable — set or unset it in the shell or secret manager that launches jira.", Retryable: false},
	CodeValidationFailed:             {Type: TypeValidation, Exit: 3, Hint: "Check the values you passed and try again.", Retryable: false},
	CodeJiraBadRequest:               {Type: TypeValidation, Exit: 3, Hint: "Jira rejected some of the values — fix the fields it flagged and send it again.", Retryable: false},
	CodeJiraConflict:                 {Type: TypeValidation, Exit: 3, Hint: "Someone changed this issue since you loaded it — re-fetch it and try again on the latest version.", Retryable: false},
	// auth (exit 1)
	CodeJiraUnauthorized:         {Type: TypeAuth, Exit: 1, Hint: "Check the profile's credential with `jira auth status`.", Retryable: false},
	CodeJiraForbidden:            {Type: TypeAuth, Exit: 1, Hint: "Your credential works but isn't allowed here — confirm the active profile with `jira auth status`, then check your project role or token scope in Jira.", Retryable: false},
	CodeCredentialMissing:        {Type: TypeAuth, Exit: 1, Hint: "No credential is saved for this profile — run `jira auth login` to add one.", Retryable: false},
	CodeCredentialSourceConflict: {Type: TypeAuth, Exit: 1, Hint: "More than one credential is set — keep just one of the flag, the environment variable, or the stored secret.", Retryable: false},
	// A backend outage is transient — every construction site marks it
	// worth retrying.
	CodeCredentialBackendUnavailable: {Type: TypeAuth, Exit: 1, Hint: "Couldn't reach the credential store — check your OS keychain or the 1Password CLI, then try again.", Retryable: true},
	CodeCredentialMigrationFailed:    {Type: TypeAuth, Exit: 1, Hint: "The migration didn't finish — run `jira auth migrate` again.", Retryable: false},
	CodeCredentialCleanupFailed:      {Type: TypeAuth, Exit: 1, Hint: "The new credential is in place, but the old one is still there — remove it from the old backend by hand.", Retryable: false},
	CodeOnePasswordUnavailable:       {Type: TypeAuth, Exit: 1, Hint: "Couldn't use 1Password — make sure `op` is installed and your account is unlocked.", Retryable: false},
	CodeCredentialRejected:           {Type: TypeAuth, Exit: 1, Hint: "Check the email and API token at id.atlassian.com, or pass --skip-verify to store the credential without checking.", Retryable: false},
	CodeCredentialVerifyUnavailable:  {Type: TypeAuth, Exit: 1, Hint: "Jira couldn't be reached to verify the credential — try again, or pass --skip-verify to store it without checking.", Retryable: true},
	CodeOnePasswordUnsupportedBuild:  {Type: TypeAuth, Exit: 1, Hint: "This build has no 1Password support — use a source build with CGO enabled, or switch to the keyring or env backend.", Retryable: false},
	CodeKeyringUnavailable:           {Type: TypeAuth, Exit: 1, Hint: "No OS keyring is available here (common on WSL and headless Linux) — set the profile's JIRA_TOKEN_<PROFILE> variable and run `jira auth login --backend env`, or install a Secret Service such as gnome-keyring.", Retryable: false},
	CodeEnvCredentialUnset:           {Type: TypeAuth, Exit: 1, Hint: "Export the profile's JIRA_TOKEN_<PROFILE> variable with the API token (JIRA_TOKEN_DEFAULT for the default profile), then retry.", Retryable: false},
	CodeAuthFailed:                   {Type: TypeAuth, Exit: 1, Hint: "Check the credential with `jira auth status`, then sign in again with `jira auth login`.", Retryable: false},
	// not_found (exit 2)
	CodeProfileNotDefined: {Type: TypeNotFound, Exit: 2, Hint: "See your profiles with `jira config profile`, or create one with `jira auth login --profile <name>`.", Retryable: false},
	CodeProfileIncomplete: {Type: TypeNotFound, Exit: 2, Hint: "Finish setting up the profile — run `jira auth login --profile <name>` to give it a base URL.", Retryable: false},
	CodeJiraNotFound:      {Type: TypeNotFound, Exit: 2, Hint: "Double-check the identifier exists and that this account can see it.", Retryable: false},
	CodeJiraGone:          {Type: TypeNotFound, Exit: 2, Hint: "This was permanently deleted in Jira — stop referring to it.", Retryable: false},
	CodeNotFound:          {Type: TypeNotFound, Exit: 2, Hint: "Re-check the identifier — it doesn't exist, or this account can't see it.", Retryable: false},
	// rate_limit (exit 4)
	CodeJiraRateLimited: {Type: TypeRateLimit, Exit: 4, Hint: "Jira is rate-limiting you — wait for the window to reset, or raise --max-retry-wait (or JIRA_MAX_RETRY_WAIT) to keep waiting automatically.", Retryable: true},
	CodeRateLimited:     {Type: TypeRateLimit, Exit: 4, Hint: "Wait for the limit to reset, or raise --max-retry-wait (or JIRA_MAX_RETRY_WAIT) to keep waiting automatically.", Retryable: true},
	// server (exit 5; canceled and timeout pin their own exits)
	CodeJiraServerError: {Type: TypeServer, Exit: 5, Hint: "Jira hit a server-side error — wait a moment and try again.", Retryable: true},
	// CodeServerError stays retryable=false: the untyped fallback has never
	// asserted retryability. An APIError whose unmapped status lands on this
	// catch-all gets the transport retry rule from the assembler instead.
	CodeServerError: {Type: TypeServer, Exit: 5, Hint: "Jira hit an unexpected error — wait a moment and try again.", Retryable: false},
	CodeCanceled:    {Type: TypeServer, Exit: 6, Hint: "Run it again when you're ready.", Retryable: true},
	CodeTimeout:     {Type: TypeServer, Exit: 7, Hint: "It ran past the --timeout deadline — raise --timeout or try again.", Retryable: true},
}

// Lookup returns the registry row for a code. ok is false for an
// unregistered code (including [CodeUnknown]); callers then fail closed to
// the server type, exit 5, and the generic hint.
func Lookup(c Code) (Spec, bool) {
	spec, ok := registry[c]
	return spec, ok
}

// HintFor returns the code's canonical hint, or the generic
// rerun-with---debug fallback for an unregistered code — never an empty
// string.
func HintFor(c Code) string {
	if spec, ok := registry[c]; ok {
		return spec.Hint
	}
	return genericHint
}

// ExitFor returns a type's default process exit code. [TypeUnknown] and any
// unrecognized type fail closed to the server exit.
func ExitFor(t Type) int {
	switch t {
	case TypeAuth:
		return 1
	case TypeNotFound:
		return 2
	case TypeValidation:
		return 3
	case TypeRateLimit:
		return 4
	default:
		return 5
	}
}

// DefaultCode returns a type's catch-all code — the code an untyped or
// unmapped failure of that type falls back to. [TypeUnknown] falls back to
// the server catch-all.
func DefaultCode(t Type) Code {
	switch t {
	case TypeAuth:
		return CodeAuthFailed
	case TypeNotFound:
		return CodeNotFound
	case TypeValidation:
		return CodeValidationFailed
	case TypeRateLimit:
		return CodeRateLimited
	default:
		return CodeServerError
	}
}

// Codes returns every registered code as a freshly allocated, sorted,
// non-nil slice; the registry itself is never exposed or reordered.
func Codes() []Code {
	return slices.Sorted(maps.Keys(registry))
}

// Coded is the minimal contract a taxonomy-aware error satisfies: it names
// its own [Code], and the registry supplies type, exit, hint, and
// retryability. The name deliberately opts out of the -er convention —
// Code is a getter, not an action.
type Coded interface {
	error
	// Code returns the stable envelope code this error classifies as.
	Code() Code
}

// Flagger names the command-line flag a validation failure is scoped to.
// The method is FlagName, not Flag: implementers carry a Flag field.
type Flagger interface{ FlagName() string }

// Candidated carries structured disambiguation rows for the envelope's
// candidates[] field. The method is CandidateRows, not Candidates:
// implementers carry a Candidates field.
type Candidated interface{ CandidateRows() []map[string]any }
