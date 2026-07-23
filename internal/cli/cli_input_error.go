package cli

import (
	"github.com/matcra587/jira-cli/internal/errtax"
)

// CLIInputKind names which class of command-line input a CLIInputError
// reports. Each kind maps 1:1 to a stable snake_case envelope code, so an
// agent branches on Error.Code without parsing the message.
type CLIInputKind int

const (
	// InputFlagUnknown is an unrecognized --flag.
	InputFlagUnknown CLIInputKind = iota
	// InputFlagValueMissing is a flag that requires a value but got none.
	InputFlagValueMissing
	// InputFlagValueInvalid is a flag value that failed type or range parsing.
	InputFlagValueInvalid
	// InputFlagSyntaxInvalid is a malformed flag token.
	InputFlagSyntaxInvalid
	// InputRequiredFlagMissing is a required flag that was not set.
	InputRequiredFlagMissing
	// InputArgCountInvalid is the wrong number of positional arguments.
	InputArgCountInvalid
	// InputCommandUnknown is an unrecognized subcommand.
	InputCommandUnknown
	// InputArgValueInvalid is a positional argument value outside the accepted set.
	InputArgValueInvalid
	// InputIssueTypeUnknown is a --type value naming no issue type on the
	// project's create screen — resolved against the fetched list in-code, so a
	// miss is validation, not a Jira 404.
	InputIssueTypeUnknown
	// InputSavedQueryUnknown is a `search saved NAME` value that matches no
	// saved query in the queries directory — a local-file lookup, so its hint
	// points at that directory rather than at --help.
	InputSavedQueryUnknown
	// InputForceRequired is a destructive or state-wiping run refused
	// because it needs explicit --force consent: headless / agent /
	// --no-input contexts for gates with an interactive confirm fallback,
	// every context for clobber guards that never prompt.
	InputForceRequired
	// InputJQExpressionInvalid is a --jq expression gojq could not parse or
	// compile.
	InputJQExpressionInvalid
	// InputJQOutputConflict is --jq combined with an explicit
	// --output=human — the filter runs over JSON output.
	InputJQOutputConflict
)

// CLIInputError is the typed error every command-line input failure is
// wrapped in before it leaves the command layer — a bad flag, a missing
// required flag, the wrong positional-argument count, or an unknown
// command. It exists so MapError classifies the failure via errors.As
// instead of letting the substring classifier guess at an untyped Cobra
// string. Every CLIInputError is an exit-3 validation failure; the Kind
// supplies the specific code, and the errtax registry supplies the hint.
//
// The struct carries only plain strings: the pflag/cobra inspection that
// produces it happens in the command layer, so internal/cli stays free of
// any Cobra dependency.
type CLIInputError struct {
	Kind        CLIInputKind
	Message     string   // diagnosis: what failed
	Flag        string   // offending flag name (no dashes), when flag-scoped
	Suggestions []string // "did you mean" candidates, pre-formatted (e.g. "--output")
}

// NewCLIInputError builds a CLIInputError of the given kind. Callers set
// Flag and Suggestions afterward when the failure is flag-scoped.
func NewCLIInputError(kind CLIInputKind, message string) *CLIInputError {
	return &CLIInputError{Kind: kind, Message: message}
}

func (e *CLIInputError) Error() string { return e.Message }

// Code is the stable snake_case envelope code for an input-failure kind.
func (e *CLIInputError) Code() errtax.Code {
	switch e.Kind {
	case InputFlagUnknown:
		// A recognized foreign-CLI flag is not a typo: it gets its own
		// orientation code, whose hint says the flag belongs to another
		// tool rather than suggesting a spelling fix.
		if isForeignFlag(e.Flag) {
			return errtax.CodeFlagForeign
		}
		return errtax.CodeFlagUnknown
	case InputFlagValueMissing:
		return errtax.CodeFlagValueMissing
	case InputFlagValueInvalid:
		return errtax.CodeFlagValueInvalid
	case InputFlagSyntaxInvalid:
		return errtax.CodeFlagSyntaxInvalid
	case InputRequiredFlagMissing:
		return errtax.CodeRequiredFlagMissing
	case InputArgCountInvalid:
		return errtax.CodeArgCountInvalid
	case InputArgValueInvalid:
		return errtax.CodeArgValueInvalid
	case InputIssueTypeUnknown:
		return errtax.CodeIssueTypeUnknown
	case InputSavedQueryUnknown:
		return errtax.CodeSavedQueryUnknown
	case InputCommandUnknown:
		return errtax.CodeCommandUnknown
	case InputForceRequired:
		// Deliberately the generic validation code every --force gate has
		// always emitted: typing hardens classification without bumping
		// the agent-visible code contract.
		return errtax.CodeValidationFailed
	case InputJQExpressionInvalid:
		return errtax.CodeJQExpressionInvalid
	case InputJQOutputConflict:
		return errtax.CodeJQOutputConflict
	default:
		return errtax.CodeValidationFailed
	}
}

// FlagName names the offending flag when the failure is flag-scoped. The
// method is FlagName (not Flag) because Flag is a field.
func (e *CLIInputError) FlagName() string { return e.Flag }

var (
	_ errtax.Coded   = (*CLIInputError)(nil) //nolint:errcheck // compile-time interface assertion
	_ errtax.Flagger = (*CLIInputError)(nil) //nolint:errcheck // compile-time interface assertion
)
