package cli

import (
	"fmt"
	"strings"
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
)

// CLIInputError is the typed error every command-line input failure is
// wrapped in before it leaves the command layer — a bad flag, a missing
// required flag, the wrong positional-argument count, or an unknown
// command. It exists so MapError classifies the failure via errors.As
// instead of letting the substring classifier guess at an untyped Cobra
// string. Every CLIInputError is an exit-3 validation failure; the Kind
// supplies the specific code and remediation hint.
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

// code is the stable snake_case envelope code for an input-failure kind.
func (e *CLIInputError) code() string {
	switch e.Kind {
	case InputFlagUnknown:
		return "flag_unknown"
	case InputFlagValueMissing:
		return "flag_value_missing"
	case InputFlagValueInvalid:
		return "flag_value_invalid"
	case InputFlagSyntaxInvalid:
		return "flag_syntax_invalid"
	case InputRequiredFlagMissing:
		return "required_flag_missing"
	case InputArgCountInvalid:
		return "arg_count_invalid"
	case InputCommandUnknown:
		return "command_unknown"
	default:
		return "validation_failed"
	}
}

// hint is the next-action remediation for an input failure. When the
// failure carries "did you mean" candidates the hint leads with them; the
// per-kind guidance follows so the caller always has a fallback step.
func (e *CLIInputError) hint() string {
	base := e.baseHint()
	if dym := didYouMean(e.Suggestions); dym != "" {
		return dym + " " + base
	}
	return base
}

// baseHint is the per-kind remediation, independent of any suggestion.
func (e *CLIInputError) baseHint() string {
	switch e.Kind {
	case InputFlagUnknown:
		return "Remove the flag or correct its name; run the command with --help to list its flags."
	case InputFlagValueMissing:
		return "Supply the flag's value, e.g. --flag=value."
	case InputFlagValueInvalid:
		return "Pass a value of the type the flag expects; run the command with --help for its documented format."
	case InputFlagSyntaxInvalid:
		return "Use --flag=value or --flag value, with no stray characters in the flag token."
	case InputRequiredFlagMissing:
		return "Pass the required flag; run the command with --help to see which flags are mandatory."
	case InputArgCountInvalid:
		return "Pass the number of positional arguments the command expects; run the command with --help for its usage line."
	case InputCommandUnknown:
		return "Run `jira --help` to list the available commands."
	default:
		return "Review the command-line input and retry."
	}
}

// didYouMean renders a suggestion clause for the hint, or "" when there
// are no candidates.
func didYouMean(suggestions []string) string {
	switch len(suggestions) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("Did you mean %s?", suggestions[0])
	default:
		return fmt.Sprintf("Did you mean %s?", strings.Join(suggestions, " or "))
	}
}
