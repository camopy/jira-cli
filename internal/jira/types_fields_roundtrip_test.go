package jira

import (
	"encoding/json"
	"testing"
)

// A decode/encode round-trip of the fields block must preserve system fields
// the struct does not name — the bug this pins: `created` (and any other
// unmodeled system field) was silently dropped at decode, so --full/*all
// output and field projections returned null for data Jira had sent.
func TestIssueFieldsRoundTripPreservesUnmodeledSystemFields(t *testing.T) {
	wire := []byte(`{
		"summary": "s",
		"created": "2026-01-02T03:04:05.000+0000",
		"duedate": "2026-02-01",
		"resolutiondate": null,
		"customfield_10010": "Sprint 5"
	}`)
	var fields IssueFields
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if fields.Created == nil || *fields.Created != "2026-01-02T03:04:05.000+0000" {
		t.Errorf("Created = %v, want the wire timestamp", fields.Created)
	}

	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var round map[string]json.RawMessage
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("re-decode error = %v", err)
	}
	// Exact wire values, one per capture route: summary and created decode
	// into named members, duedate rides Extra, customfield_10010 rides
	// CustomFields, and resolutiondate's explicit null survives verbatim.
	for key, want := range map[string]string{
		"summary":           `"s"`,
		"created":           `"2026-01-02T03:04:05.000+0000"`,
		"duedate":           `"2026-02-01"`,
		"customfield_10010": `"Sprint 5"`,
		"resolutiondate":    `null`,
	} {
		got, ok := round[key]
		if !ok {
			t.Errorf("round-trip dropped %q; got keys %v", key, keysOf(round))
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
	// A named field must never be duplicated or overridden by the raw
	// passthrough: summary decodes into the struct, not the extras.
	if fields.Summary == nil || *fields.Summary != "s" {
		t.Errorf("Summary = %v, want s", fields.Summary)
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
