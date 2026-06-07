package cli

import (
	"testing"
	"unicode"
)

// TestOperationVerbsAreLowerCase locks the house standard: every verb form is a
// lower-case event string (clog's own lines and the completion messages are
// lower case). The noun may carry an acronym (e.g. "JQL reference"), so only the
// gerund and past forms are checked.
func TestOperationVerbsAreLowerCase(t *testing.T) {
	firstUpper := func(s string) bool {
		for _, r := range s {
			return unicode.IsUpper(r)
		}
		return false
	}
	for op, v := range operationVerbs {
		if firstUpper(v.Gerund) {
			t.Errorf("operationVerbs[%q].Gerund = %q, want lower case", op, v.Gerund)
		}
		if firstUpper(v.Past) {
			t.Errorf("operationVerbs[%q].Past = %q, want lower case", op, v.Past)
		}
		if firstUpper(v.Infinitive) {
			t.Errorf("operationVerbs[%q].Infinitive = %q, want lower case", op, v.Infinitive)
		}
	}
}

// TestCompletionMessagesComeFromTheRegistry pins the single source of truth:
// every command that shows a completion line derives its phrase from the verb
// registry, so the spinner, the debug lifecycle, and the completion message can
// never drift apart.
func TestCompletionMessagesComeFromTheRegistry(t *testing.T) {
	for op := range completionMessageCommands {
		if _, ok := operationVerbs[op]; !ok {
			t.Errorf("completion command %q has no entry in operationVerbs", op)
		}
		// The completion line is the Sentence-cased past-tense phrase from the
		// registry (user-facing UI), while the registry itself stays lower case.
		if got, want := messageForCommand(op), SentenceCase(VerbFor(op).Pastf()); got != want {
			t.Errorf("messageForCommand(%q) = %q, want %q", op, got, want)
		}
	}
	if got := messageForCommand("auth.whoami"); got != "" {
		t.Errorf("messageForCommand(auth.whoami) = %q, want empty", got)
	}
}
