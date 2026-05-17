package jira

import "testing"

func TestNextOffsetUsesPageSizeForPartialPages(t *testing.T) {
	if got, want := nextOffset(0, 1, 50, 50), 50; got != want {
		t.Fatalf("nextOffset() = %d, want %d", got, want)
	}
}

func TestNextOffsetAdvancesEmptyNonFinalPages(t *testing.T) {
	if got, want := nextOffset(50, 0, 50, 50), 100; got != want {
		t.Fatalf("nextOffset() = %d, want %d", got, want)
	}
}
