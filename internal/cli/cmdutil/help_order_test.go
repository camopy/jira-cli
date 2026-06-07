package cmdutil

import (
	"testing"

	"github.com/gechr/clib/help"
)

// fgSection builds a flag-group section the way clib does — a titled section
// carrying a FlagGroup — so orderFlagGroups recognizes it as reorderable.
func fgSection(title string) help.Section {
	return help.Section{Title: title, Content: []help.Content{help.FlagGroup{}}}
}

// orderFlagGroups sorts flag-group sections into the canonical task-flow order
// while leaving structural sections (Usage, Examples) in place.
func TestOrderFlagGroups(t *testing.T) {
	titles := func(sections []help.Section) []string {
		out := make([]string, len(sections))
		for i, s := range sections {
			out[i] = s.Title
		}
		return out
	}

	t.Run("domain groups sort into task-flow order, Usage stays first", func(t *testing.T) {
		in := []help.Section{
			{Title: "Usage"},
			fgSection("Output"),
			fgSection("Filters"),
			fgSection("Sort"),
			fgSection("Options"),
		}
		got := titles(orderFlagGroups(in))
		want := []string{"Usage", "Filters", "Sort", "Output", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("Execution sinks below Output as a niche tuning group", func(t *testing.T) {
		in := []help.Section{
			fgSection("Filters"),
			fgSection("Sort"),
			fgSection("Execution"),
			fgSection("Output"),
			fgSection("Options"),
		}
		got := titles(orderFlagGroups(in))
		want := []string{"Filters", "Sort", "Output", "Execution", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("inherited Options / Global Options render last", func(t *testing.T) {
		in := []help.Section{fgSection("Global Options"), fgSection("Output"), fgSection("Filters")}
		got := titles(orderFlagGroups(in))
		want := []string{"Filters", "Output", "Global Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("an unranked group sinks to just before Options", func(t *testing.T) {
		in := []help.Section{fgSection("Output"), fgSection("Mystery"), fgSection("Options"), fgSection("Filters")}
		got := titles(orderFlagGroups(in))
		want := []string{"Filters", "Output", "Mystery", "Options"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("structural sections never move", func(t *testing.T) {
		in := []help.Section{
			{Title: "Usage"},
			fgSection("Output"),
			{Title: "Examples"},
			fgSection("Filters"),
		}
		got := titles(orderFlagGroups(in))
		want := []string{"Usage", "Filters", "Examples", "Output"}
		if !equalStrings(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("fewer than two flag groups is a no-op", func(t *testing.T) {
		in := []help.Section{{Title: "Usage"}, fgSection("Filters")}
		got := titles(orderFlagGroups(in))
		want := []string{"Usage", "Filters"}
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
