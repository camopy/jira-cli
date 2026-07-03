package cli

import (
	"strings"
	"testing"
)

// A flag borrowed from another Jira CLI produces a teaching hint that
// names the origin and this CLI's actual contract — correcting the
// caller's mental model, not just the attempt.

func TestForeignFlagHintTeachesKnownFlags(t *testing.T) {
	e := NewCLIInputError(InputFlagUnknown, "unknown flag: --plain")
	e.Flag = "plain"
	hint := e.hint()
	for _, want := range []string{"ankitpokhrel/jira-cli", "--output=human", "--output=json", "core_contract"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("teaching hint missing %q: %s", want, hint)
		}
	}
}

func TestForeignFlagHintToleratesDashesAndCase(t *testing.T) {
	e := NewCLIInputError(InputFlagUnknown, "unknown flag: --Plain")
	e.Flag = "--Plain"
	if !strings.Contains(e.hint(), "--output=human") {
		t.Fatalf("dash/case drift must still resolve the foreign flag: %s", e.hint())
	}
}

func TestForeignFlagHintLeavesUnknownFlagsGeneric(t *testing.T) {
	e := NewCLIInputError(InputFlagUnknown, "unknown flag: --frobnicate")
	e.Flag = "frobnicate"
	hint := e.hint()
	if !strings.Contains(hint, "Remove the flag or correct its name") {
		t.Fatalf("non-foreign unknown flags keep the generic remediation: %s", hint)
	}
	if strings.Contains(hint, "ankitpokhrel") {
		t.Fatalf("no foreign attribution for an unrecognized flag: %s", hint)
	}
}

func TestForeignFlagHintSuggestionStillLeads(t *testing.T) {
	// A did-you-mean candidate is more actionable than the teaching
	// sentence and keeps its lead position.
	e := NewCLIInputError(InputFlagUnknown, "unknown flag: --plain")
	e.Flag = "plain"
	e.Suggestions = []string{"--parent"}
	hint := e.hint()
	if !strings.HasPrefix(hint, "Did you mean --parent?") {
		t.Fatalf("suggestion must lead: %s", hint)
	}
	if !strings.Contains(hint, "ankitpokhrel/jira-cli") {
		t.Fatalf("teaching sentence must still follow: %s", hint)
	}
}
