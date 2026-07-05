package jira

import (
	"strings"
	"testing"
	"time"
)

// The fixed reference clock and zone keep every case deterministic: a naive
// or relative value must resolve against these, never the host machine.
var (
	startedTestLoc = time.FixedZone("UTC-4", -4*3600)
	startedTestNow = time.Date(2026, 6, 26, 15, 30, 45, 0, startedTestLoc)
)

func TestParseStartedNormalizesAcceptedForms(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"rfc3339 utc", "2026-06-26T10:00:00Z", "2026-06-26T10:00:00.000+0000"},
		{"rfc3339 offset kept", "2026-06-26T10:00:00+02:00", "2026-06-26T10:00:00.000+0200"},
		{"jira strict form unchanged", "2026-05-03T09:30:00.000-0400", "2026-05-03T09:30:00.000-0400"},
		{"fractional seconds truncated to millis", "2026-06-26T10:00:00.123456Z", "2026-06-26T10:00:00.123+0000"},
		{"no-colon offset without millis", "2026-06-26T10:00:00+0000", "2026-06-26T10:00:00.000+0000"},
		{"naive local seconds", "2026-06-26T10:00:00", "2026-06-26T10:00:00.000-0400"},
		{"naive local minutes", "2026-06-26T10:00", "2026-06-26T10:00:00.000-0400"},
		{"naive local space separator", "2026-06-26 10:00:00", "2026-06-26T10:00:00.000-0400"},
		{"bare date is local midnight", "2026-06-26", "2026-06-26T00:00:00.000-0400"},
		{"now", "now", "2026-06-26T15:30:45.000-0400"},
		{"yesterday keeps the time of day", "yesterday", "2026-06-25T15:30:45.000-0400"},
		{"duration ago", "2h ago", "2026-06-26T13:30:45.000-0400"},
		{"compound duration ago", "1d2h ago", "2026-06-25T13:30:45.000-0400"},
		{"relative is case-insensitive", "2H AGO", "2026-06-26T13:30:45.000-0400"},
		{"keyword is case-insensitive", "Yesterday", "2026-06-25T15:30:45.000-0400"},
		{"surrounding whitespace trimmed", "  2026-06-26T10:00  ", "2026-06-26T10:00:00.000-0400"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStarted(tc.input, startedTestNow, startedTestLoc)
			if err != nil {
				t.Fatalf("ParseStarted(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseStarted(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseStartedRejectsUnparseableValues(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"natural language", "tomorrow"},
		{"free text", "last tuesday morning"},
		{"bare duration without ago", "2h"},
		{"negative relative", "-2h ago"},
		{"unknown relative unit", "2fortnights ago"},
		{"empty", ""},
		{"out-of-range date", "2026-02-30T10:00:00"},
		{"time only", "10:00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := ParseStarted(tc.input, startedTestNow, startedTestLoc); err == nil {
				t.Fatalf("ParseStarted(%q) = %q, want error", tc.input, got)
			}
		})
	}
}

// Every accepted form must land on Jira's strict layout — round-tripping the
// output through that single layout is the wire-format guarantee.
func TestParseStartedOutputAlwaysMatchesJiraLayout(t *testing.T) {
	inputs := []string{
		"2026-06-26T10:00:00Z",
		"2026-06-26T10:00",
		"2026-06-26",
		"now",
		"yesterday",
		"90m ago",
	}
	for _, input := range inputs {
		got, err := ParseStarted(input, startedTestNow, startedTestLoc)
		if err != nil {
			t.Fatalf("ParseStarted(%q) error = %v", input, err)
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000-0700", got); err != nil {
			t.Fatalf("ParseStarted(%q) = %q does not match Jira's started layout: %v", input, got, err)
		}
		if strings.Contains(got, "Z") {
			t.Fatalf("ParseStarted(%q) = %q carries a Z suffix; Jira needs a numeric offset", input, got)
		}
	}
}
