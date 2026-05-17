package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestParseWorklogDurations(t *testing.T) {
	tests := map[string]int{
		"2h30m": 9000,
		"1d":    28800,
		"45m":   2700,
	}
	for input, want := range tests {
		got, err := jira.ParseDuration(input, 28800)
		if err != nil || got != want {
			t.Fatalf("ParseDuration(%q) = %d, %v", input, got, err)
		}
	}
	for _, input := range []string{"0m", "-1h", "3w"} {
		if _, err := jira.ParseDuration(input, 28800); err == nil {
			t.Fatalf("ParseDuration(%q) error = nil", input)
		}
	}
}
