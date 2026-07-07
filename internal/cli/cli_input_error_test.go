package cli

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// allCLIInputKinds is every kind the taxonomy defines. A test that
// iterates it fails to compile-pass silently when a kind is added without
// being considered here.
var allCLIInputKinds = []CLIInputKind{
	InputFlagUnknown,
	InputFlagValueMissing,
	InputFlagValueInvalid,
	InputFlagSyntaxInvalid,
	InputRequiredFlagMissing,
	InputArgCountInvalid,
	InputArgValueInvalid,
	InputCommandUnknown,
}

// TestEveryCLIInputKindCarriesACodeAndHint pins the invariant that every
// kind resolves to a specific registered code (never the generic
// validation_failed fallback) whose registry row carries a non-empty
// remediation hint. A kind that falls through either branch hands an
// agent a failure with no stable code to branch on or no next step.
func TestEveryCLIInputKindCarriesACodeAndHint(t *testing.T) {
	for _, kind := range allCLIInputKinds {
		e := NewCLIInputError(kind, "input failed")
		code := e.Code()
		if code == errtax.CodeUnknown || code == errtax.CodeValidationFailed {
			t.Errorf("kind %d resolves to %q, not a specific cli-input code", kind, code)
		}
		spec, ok := errtax.Lookup(code)
		if !ok || spec.Hint == "" {
			t.Errorf("kind %d (code %q) has no registry hint", kind, code)
		}
	}
}

// TestCLIInputCodesAreStable locks the snake_case code strings: they are
// the agent-facing contract and must not drift without a deliberate edit
// to this expectation.
func TestCLIInputCodesAreStable(t *testing.T) {
	want := map[CLIInputKind]string{
		InputFlagUnknown:         "flag_unknown",
		InputFlagValueMissing:    "flag_value_missing",
		InputFlagValueInvalid:    "flag_value_invalid",
		InputFlagSyntaxInvalid:   "flag_syntax_invalid",
		InputRequiredFlagMissing: "required_flag_missing",
		InputArgCountInvalid:     "arg_count_invalid",
		InputArgValueInvalid:     "arg_value_invalid",
		InputCommandUnknown:      "command_unknown",
	}
	if len(want) != len(allCLIInputKinds) {
		t.Fatalf("want map has %d entries, taxonomy has %d", len(want), len(allCLIInputKinds))
	}
	for kind, code := range want {
		if got := string(NewCLIInputError(kind, "x").Code()); got != code {
			t.Errorf("kind %d: code = %q, want %q", kind, got, code)
		}
	}
}

// TestMapCLIInputErrorThroughMapError verifies the MapError leg: a
// *CLIInputError becomes an exit-3 validation envelope carrying the typed
// code, the static registry hint, the offending flag, and the "did you
// mean" candidates in the suggestions field — never woven into the hint.
func TestMapCLIInputErrorThroughMapError(t *testing.T) {
	in := NewCLIInputError(InputFlagUnknown, "unknown flag: --outpt")
	in.Flag = "outpt"
	in.Suggestions = []string{"--output"}

	got := MapError(in)
	if got.Type != string(ErrorTypeValidation) {
		t.Errorf("Type = %q, want %q", got.Type, ErrorTypeValidation)
	}
	if got.Code != "flag_unknown" {
		t.Errorf("Code = %q, want flag_unknown", got.Code)
	}
	if got.Flag != "outpt" {
		t.Errorf("Flag = %q, want outpt", got.Flag)
	}
	if got.Hint != errtax.HintFor(errtax.CodeFlagUnknown) {
		t.Errorf("Hint = %q, want the static registry hint", got.Hint)
	}
	if strings.Contains(got.Hint, "Did you mean") {
		t.Errorf("hint must not embed suggestions, got %q", got.Hint)
	}
	if len(got.Suggestions) != 1 || got.Suggestions[0] != "--output" {
		t.Errorf("Suggestions = %v, want [--output]", got.Suggestions)
	}
	if got.Retryable {
		t.Error("a cli-input failure must not be retryable")
	}
	if exit := ExitCode(got); exit != 3 {
		t.Errorf("ExitCode = %d, want 3", exit)
	}
}
