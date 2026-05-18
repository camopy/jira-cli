package cli

import (
	"reflect"
	"testing"
)

func TestSuggest(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		candidates []string
		want       []string
	}{
		{
			name:       "single-character typo within distance",
			input:      "outpt",
			candidates: []string{"output", "timeout", "color"},
			want:       []string{"output"},
		},
		{
			name:       "transposition within distance two",
			input:      "porject",
			candidates: []string{"project", "priority", "profile"},
			want:       []string{"project"},
		},
		{
			name:       "distance beyond threshold is rejected",
			input:      "xyz",
			candidates: []string{"output", "timeout"},
			want:       []string{},
		},
		{
			name:       "candidates shorter than the minimum are skipped",
			input:      "abc",
			candidates: []string{"abd", "abe"},
			want:       []string{},
		},
		{
			name:       "exact match is not suggested back",
			input:      "output",
			candidates: []string{"output"},
			want:       []string{},
		},
		{
			name:       "closest candidate is ordered first",
			input:      "aaaa",
			candidates: []string{"aabc", "aaab"},
			want:       []string{"aaab", "aabc"},
		},
		{
			name:       "result is capped at the suggestion limit",
			input:      "fxo",
			candidates: []string{"foo1", "foo2", "foo3", "foo4"},
			want:       []string{"foo1", "foo2"},
		},
		{
			name:       "empty input yields nothing",
			input:      "",
			candidates: []string{"output"},
			want:       nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Suggest(c.input, c.candidates)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Suggest(%q, %v) = %v, want %v", c.input, c.candidates, got, c.want)
			}
		})
	}
}
