package cli

import (
	"bytes"
	"strings"
	"testing"
)

type nilPlainStringer struct{}

func (*nilPlainStringer) String() string {
	panic("nil Stringer should not be invoked")
}

func TestGenericPlainCollapsesComplexValues(t *testing.T) {
	type issueSummary struct {
		Key    string `json:"key"`
		Status string `json:"status"`
	}

	var missing *nilPlainStringer
	var buf bytes.Buffer
	err := WritePlain(&buf, map[string]any{
		"body": "first line\nsecond line",
		"issue": issueSummary{
			Key:    "JCT-61",
			Status: "To Do",
		},
		"link_types": []any{
			map[string]any{"id": "10000", "name": "Blocks"},
			map[string]any{"id": "10001", "name": "Relates"},
		},
		"missing": missing,
	})
	if err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}

	got := buf.String()
	for _, bad := range []string{`[{`, `{\"`, `\"id\"`, `\nsecond line`} {
		if strings.Contains(got, bad) {
			t.Fatalf("generic plain output still contains escaped complex value %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{"link_types", "[2 items]", "issue", "{...}", "body", "first line second line"} {
		if !strings.Contains(got, want) {
			t.Fatalf("generic plain output missing %q:\n%s", want, got)
		}
	}
}

func TestMutationPlainPayloadsDoNotUseListRenderers(t *testing.T) {
	tests := []struct {
		name    string
		command string
		data    map[string]any
		bad     string
		want    []string
	}{
		{
			name:    "watcher add no readback",
			command: "issue.watchers.add",
			data: map[string]any{
				"account_id": "abc-123",
				"attempted":  true,
			},
			bad:  "(no watchers visible)",
			want: []string{"account_id=abc-123", "attempted=true"},
		},
		{
			name:    "attachment add dry run",
			command: "issue.attachment.add",
			data: map[string]any{
				"key":     "PROJ-1",
				"dry_run": true,
				"files": []map[string]any{
					{"path": "trace.log", "size": 12},
				},
			},
			bad:  "(no attachments)",
			want: []string{"key=PROJ-1", "dry_run=true", "files=\"[1 item]\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteCommandPlain(&buf, tt.command, tt.data); err != nil {
				t.Fatalf("WriteCommandPlain() error = %v", err)
			}
			got := buf.String()
			if strings.Contains(got, tt.bad) {
				t.Fatalf("%s used list renderer and lost mutation payload:\n%s", tt.command, got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("%s output missing %q:\n%s", tt.command, want, got)
				}
			}
		})
	}
}
