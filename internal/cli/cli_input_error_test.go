package cli

import (
	"strings"
	"testing"
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
// kind resolves to a specific code (never the generic validation_failed
// fallback) and a non-empty remediation hint. A kind that falls through
// either branch hands an agent a failure with no stable code to branch on
// or no next step — the exact gaps this typed-error layer exists to close.
func TestEveryCLIInputKindCarriesACodeAndHint(t *testing.T) {
	for _, kind := range allCLIInputKinds {
		e := NewCLIInputError(kind, "input failed")
		code := e.code()
		if code == "" || code == "validation_failed" {
			t.Errorf("kind %d resolves to %q, not a specific cli-input code", kind, code)
		}
		if hint := e.hint(); hint == "" {
			t.Errorf("kind %d (code %q) yields no hint", kind, code)
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
		if got := NewCLIInputError(kind, "x").code(); got != code {
			t.Errorf("kind %d: code = %q, want %q", kind, got, code)
		}
	}
}

// TestCLIInputHintLeadsWithSuggestion verifies a "did you mean" candidate
// is surfaced ahead of the per-kind guidance, and that the guidance still
// follows so the caller always has a fallback step.
func TestCLIInputHintLeadsWithSuggestion(t *testing.T) {
	e := NewCLIInputError(InputFlagUnknown, "unknown flag: --outpt")
	e.Suggestions = []string{"--output"}
	hint := e.hint()
	if !strings.HasPrefix(hint, "Did you mean --output?") {
		t.Errorf("hint should lead with the suggestion, got %q", hint)
	}
	if !strings.Contains(hint, e.baseHint()) {
		t.Errorf("hint should still carry the per-kind guidance, got %q", hint)
	}

	noSuggest := NewCLIInputError(InputFlagUnknown, "unknown flag: --outpt")
	if got := noSuggest.hint(); got != noSuggest.baseHint() {
		t.Errorf("hint without suggestions = %q, want bare baseHint", got)
	}
}

func TestDidYouMean(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"--output"}, "Did you mean --output?"},
		{[]string{"--output", "--timeout"}, "Did you mean --output or --timeout?"},
	}
	for _, c := range cases {
		if got := didYouMean(c.in); got != c.want {
			t.Errorf("didYouMean(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestMapCLIInputErrorThroughMapError verifies the MapError leg: a
// *CLIInputError becomes an exit-3 validation envelope carrying the typed
// code, the hint, and the offending flag — never the substring classifier.
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
	if !strings.Contains(got.Hint, "--output") {
		t.Errorf("Hint should carry the suggestion, got %q", got.Hint)
	}
	if got.Retryable {
		t.Error("a cli-input failure must not be retryable")
	}
	if exit := ExitCode(got); exit != 3 {
		t.Errorf("ExitCode = %d, want 3", exit)
	}
}
