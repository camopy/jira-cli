package cli

import (
	"slices"
	"testing"
)

// A flag borrowed from another Jira CLI has no near-miss in this CLI's
// flag set, so the unknown-flag path offers that CLI's actual equivalents
// as suggestions — correcting the caller's vocabulary, not just the
// spelling.

func TestForeignFlagSuggestionsKnownFlag(t *testing.T) {
	want := []string{"--output=human", "--output=json"}
	if got := ForeignFlagSuggestions("plain"); !slices.Equal(got, want) {
		t.Fatalf("ForeignFlagSuggestions(plain) = %v, want %v", got, want)
	}
}

func TestForeignFlagSuggestionsToleratesDashesAndCase(t *testing.T) {
	want := []string{"--output=human", "--output=json"}
	if got := ForeignFlagSuggestions("--Plain"); !slices.Equal(got, want) {
		t.Fatalf("dash/case drift must still resolve the foreign flag: %v", got)
	}
}

func TestForeignFlagSuggestionsUnknownFlagIsNil(t *testing.T) {
	if got := ForeignFlagSuggestions("frobnicate"); got != nil {
		t.Fatalf("no equivalents expected for an unrecognized flag, got %v", got)
	}
}

func TestForeignFlagSuggestionsReturnsAFreshSlice(t *testing.T) {
	first := ForeignFlagSuggestions("gjq")
	first[0] = "mutated"
	if second := ForeignFlagSuggestions("gjq"); second[0] == "mutated" {
		t.Fatal("ForeignFlagSuggestions shares a backing array across calls")
	}
}
