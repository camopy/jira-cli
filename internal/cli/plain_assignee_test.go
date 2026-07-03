package cli

import (
	"bytes"
	"strings"
	"testing"
)

// A user object rendered into a human cell always resolves to a display
// string; Go's fmt map rendering ("map[]") must never leak into a table.

func TestFormatAssigneeNeverLeaksMapRendering(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, ""},
		{"empty map (unassigned)", map[string]any{}, ""},
		{"identity-less map", map[string]any{"active": true}, ""},
		{"wire displayName", map[string]any{"displayName": "Alice"}, "Alice"},
		{"normalized display_name", map[string]any{"display_name": "Bob"}, "Bob"},
		{"account id fallback", map[string]any{"accountId": "abc-123"}, "abc-123"},
		{"string passthrough", "Carol", "Carol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatAssignee(tc.value); got != tc.want {
				t.Fatalf("formatAssignee(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestIssueViewManyUnassignedRendersAsUnassigned(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"results": []any{
			map[string]any{"key": "PROJ-1", "ok": true, "issue": map[string]any{
				"key": "PROJ-1",
				"fields": map[string]any{
					"summary":  "no assignee here",
					"status":   map[string]any{"name": "Backlog"},
					"assignee": map[string]any{},
					"priority": map[string]any{"name": "Lowest"},
				},
			}},
			map[string]any{"key": "PROJ-2", "ok": true, "issue": map[string]any{
				"key": "PROJ-2",
				"fields": map[string]any{
					"summary":  "assigned",
					"status":   map[string]any{"name": "Done"},
					"assignee": map[string]any{"displayName": "Alice"},
					"priority": map[string]any{"name": "High"},
				},
			}},
		},
		"succeeded": 2,
		"failed":    0,
	}
	if err := WriteIssueViewPlain(&buf, "issue.view", data); err != nil {
		t.Fatalf("WriteIssueViewPlain: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "map[") {
		t.Fatalf("Go map rendering leaked into the table:\n%s", got)
	}
	if !strings.Contains(got, "unassigned") {
		t.Fatalf("empty assignee must render as unassigned:\n%s", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Fatalf("assigned row must show the display name:\n%s", got)
	}
}
