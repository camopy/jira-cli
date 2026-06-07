package cli_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestVerbForPhrases(t *testing.T) {
	v := cli.VerbFor("issue.create")
	if got := v.Gerundf(); got != "creating issue" {
		t.Errorf("Gerundf = %q, want %q", got, "creating issue")
	}
	if got := v.Pastf(); got != "created issue" {
		t.Errorf("Pastf = %q, want %q", got, "created issue")
	}
	if got := v.Failuref(); got != "failed to create issue" {
		t.Errorf("Failuref = %q, want %q", got, "failed to create issue")
	}
}

func TestVerbForDestructiveGerund(t *testing.T) {
	for op, want := range map[string]string{
		"issue.delete": "deleting",
		"issue.clone":  "cloning",
		"issue.move":   "moving",
	} {
		if got := cli.VerbFor(op).Gerund; got != want {
			t.Errorf("VerbFor(%q).Gerund = %q, want %q", op, got, want)
		}
	}
}

func TestVerbForPluralBatchPhrases(t *testing.T) {
	v := cli.VerbFor("issue.view") // noun "issue" -> "issues"
	if got := v.GerundPlural(); got != "viewing issues" {
		t.Errorf("GerundPlural = %q, want %q", got, "viewing issues")
	}
	if got := v.PastPlural(); got != "viewed issues" {
		t.Errorf("PastPlural = %q, want %q", got, "viewed issues")
	}
	// An already-plural noun is not double-pluralised.
	if got := cli.VerbFor("issue.list").PastPlural(); got != "listed issues" {
		t.Errorf("PastPlural(issue.list) = %q, want %q", got, "listed issues")
	}
}

func TestVerbForCacheBoards(t *testing.T) {
	v := cli.VerbFor("cache.boards")
	if v.Gerundf() != "caching boards" || v.Pastf() != "cached boards" {
		t.Errorf("cache.boards = %q / %q, want caching boards / cached boards", v.Gerundf(), v.Pastf())
	}
}

func TestVerbForUnknownFallsBackToProcessing(t *testing.T) {
	v := cli.VerbFor("widget.frobnicate")
	if v.Gerund != "processing" || v.Noun != "frobnicate" {
		t.Errorf("unknown fallback = %+v, want processing/frobnicate", v)
	}
}
