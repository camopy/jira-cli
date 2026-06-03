package completion

import (
	"slices"
	"testing"
)

func TestUniqueCachedNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []namedCacheValue
		want []string
	}{
		{
			name: "collapses per-workflow duplicates in first-seen order",
			in: []namedCacheValue{
				{Name: "To Do"},
				{Name: "In Progress"},
				{Name: "Done"},
				{Name: "To Do"},
				{Name: "In Progress"},
				{Name: "Done"},
				{Name: "To Do"},
			},
			want: []string{"To Do", "In Progress", "Done"},
		},
		{
			name: "drops blank names",
			in:   []namedCacheValue{{Name: "High"}, {Name: ""}, {Name: "Low"}},
			want: []string{"High", "Low"},
		},
		{
			name: "passes a unique list through unchanged",
			in:   []namedCacheValue{{Name: "Highest"}, {Name: "High"}, {Name: "Medium"}},
			want: []string{"Highest", "High", "Medium"},
		},
		{
			name: "empty input yields an empty, non-nil slice",
			in:   nil,
			want: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueCachedNames(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("uniqueCachedNames = %v, want %v", got, tc.want)
			}
		})
	}
}
