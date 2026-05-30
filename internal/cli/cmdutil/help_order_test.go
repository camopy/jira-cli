package cmdutil

import (
	"testing"

	"github.com/gechr/clib/help"
)

// moveOutputSectionLast pins the Output flag group to the end while preserving
// the relative order of the other sections.
func TestMoveOutputSectionLast(t *testing.T) {
	titles := func(sections []help.Section) []string {
		out := make([]string, len(sections))
		for i, s := range sections {
			out[i] = s.Title
		}
		return out
	}

	t.Run("Output becomes the last domain group, before Options", func(t *testing.T) {
		in := []help.Section{
			{Title: "Usage"},
			{Title: "Output"},
			{Title: "Filters"},
			{Title: "Options"},
		}
		got := titles(moveOutputSectionLast(in))
		want := []string{"Usage", "Filters", "Output", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("Output goes to the very end when there is no Options block", func(t *testing.T) {
		in := []help.Section{{Title: "Usage"}, {Title: "Output"}, {Title: "Filters"}}
		got := titles(moveOutputSectionLast(in))
		want := []string{"Usage", "Filters", "Output"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("already in place is unchanged", func(t *testing.T) {
		in := []help.Section{{Title: "Filters"}, {Title: "Output"}, {Title: "Options"}}
		got := titles(moveOutputSectionLast(in))
		want := []string{"Filters", "Output", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("no Output section is a no-op", func(t *testing.T) {
		in := []help.Section{{Title: "Usage"}, {Title: "Filters"}, {Title: "Options"}}
		got := titles(moveOutputSectionLast(in))
		want := []string{"Usage", "Filters", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
