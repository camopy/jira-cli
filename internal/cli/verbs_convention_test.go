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
		// registry (user-facing UI), while the registry itself stays lower
		// case; a dry-run payload flips it to the conditional form.
		if got, want := messageForCommand(op, nil), SentenceCase(VerbFor(op).Pastf()); got != want {
			t.Errorf("messageForCommand(%q) = %q, want %q", op, got, want)
		}
		dry := map[string]any{"dry_run": true}
		if got, want := messageForCommand(op, dry), SentenceCase(VerbFor(op).Conditionalf()); got != want {
			t.Errorf("messageForCommand(%q, dry-run) = %q, want %q", op, got, want)
		}
		// A payload with no dry_run key at all (every read command) keeps
		// the past-tense form.
		if got, want := messageForCommand(op, map[string]any{}), SentenceCase(VerbFor(op).Pastf()); got != want {
			t.Errorf("messageForCommand(%q, no dry_run key) = %q, want %q", op, got, want)
		}
	}
	if got := messageForCommand("auth.whoami", nil); got != "" {
		t.Errorf("messageForCommand(auth.whoami) = %q, want empty", got)
	}
}

// TestPreviewVerbFormsStayGrammatical pins the dry-run lifecycle rework:
// the noun and infinitive collapse into a compound noun so every phrase
// form reads correctly for a preview.
func TestPreviewVerbFormsStayGrammatical(t *testing.T) {
	v := VerbFor("issue.edit").Preview()
	cases := []struct{ name, got, want string }{
		{"gerund", v.Gerundf(), "previewing issue edit"},
		{"past", v.Pastf(), "previewed issue edit"},
		{"failure", v.Failuref(), "failed to preview issue edit"},
		{"conditional", v.Conditionalf(), "would preview issue edit"},
		{"gerund plural", v.GerundPlural(), "previewing issue edits"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("preview %s form = %q, want %q", c.name, c.got, c.want)
		}
	}
}
