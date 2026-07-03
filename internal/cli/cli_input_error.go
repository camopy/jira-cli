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
	// InputArgValueInvalid is a positional argument value outside the accepted set.
	InputArgValueInvalid
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
	case InputArgValueInvalid:
		return "arg_value_invalid"
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

// foreignFlagHints teaches a caller arriving with another Jira CLI's
// muscle memory: each entry names the flag's origin and this CLI's
// actual contract. Only flags with NO equivalent anywhere in this CLI
// belong here — a flag that exists on some commands (raw, columns,
// web) would produce a misleading scope claim, and a flag the failing
// command accepts never reaches flag_unknown at all.
var foreignFlagHints = map[string]string{
	"plain":       "--plain is an ankitpokhrel/jira-cli flag; this CLI prints human text with --output=human and machine JSON with --output=json.",
	"gjq":         "--gjq is an ankitpokhrel/jira-cli flag; this CLI has no built-in query language — pipe --output=json through jq instead.",
	"template":    "--template is a go-jira flag; this CLI has no template output — use --output=json and format downstream.",
	"no-headers":  "--no-headers is an ankitpokhrel/jira-cli flag; use --output=json for machine-stable output.",
	"no-truncate": "--no-truncate is an ankitpokhrel/jira-cli flag; --output=json never truncates.",
	"paginate":    "--paginate is an ankitpokhrel/jira-cli flag; page with --limit/--all and resume via --cursor from meta.pagination.nextCursor.",
}

// foreignFlagHint resolves a flag name against the foreign-CLI table,
// tolerating leading dashes and case drift in how the parser reported it.
func foreignFlagHint(flag string) string {
	name := strings.ToLower(strings.TrimLeft(strings.TrimSpace(flag), "-"))
	if hint, ok := foreignFlagHints[name]; ok {
		return hint + " Run `jira agent guide core_contract` for the output contract."
	}
	return ""
}

// baseHint is the per-kind remediation, independent of any suggestion.
func (e *CLIInputError) baseHint() string {
	switch e.Kind {
	case InputFlagUnknown:
		// A flag borrowed from another Jira CLI gets a teaching hint —
		// correcting the caller's mental model, not just the attempt.
		if foreign := foreignFlagHint(e.Flag); foreign != "" {
			return foreign
		}
		return "Remove the flag or correct its name; run the command with --help to list its flags, or read `jira agent schema --output=compact` for the machine-readable surface."
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
	case InputArgValueInvalid:
		return "Pass one of the documented positional argument values; run the command with --help for valid choices."
	case InputCommandUnknown:
		return "Run `jira --help` to list the available commands, or read `jira agent schema --output=compact` for the live machine-readable command surface."
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
