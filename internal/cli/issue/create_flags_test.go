package issue

import (
	"reflect"
	"testing"
)

// applyCreateFlags merges the convenience flags into the create payload using
// the create-input aliases (project_key/issue_type) and Jira wire shapes
// (parent.key, priority.name, labels array). Empty flags leave the payload
// untouched so they override --json-input only when actually supplied.
func TestApplyCreateFlags(t *testing.T) {
	t.Run("sets every supplied field", func(t *testing.T) {
		payload := map[string]any{"summary": "S"}
		applyCreateFlags(payload, createFlags{
			project:   "ACME",
			issueType: "Bug",
			parent:    "ACME-1",
			priority:  "High",
			labels:    []string{"alpha", "  beta  ", "", "   "},
		})
		want := map[string]any{
			"summary":     "S",
			"project_key": "ACME",
			"issue_type":  "Bug",
			"parent":      map[string]any{"key": "ACME-1"},
			"priority":    map[string]any{"name": "High"},
			"labels":      []string{"alpha", "beta"},
		}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("payload = %#v\nwant %#v", payload, want)
		}
	})

	t.Run("empty flags leave json-input values untouched", func(t *testing.T) {
		payload := map[string]any{
			"project_key": "FROMJSON",
			"priority":    map[string]any{"name": "Low"},
		}
		applyCreateFlags(payload, createFlags{labels: []string{"   "}})
		want := map[string]any{
			"project_key": "FROMJSON",
			"priority":    map[string]any{"name": "Low"},
		}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("payload = %#v\nwant %#v", payload, want)
		}
	})

	t.Run("set flag overrides json-input value", func(t *testing.T) {
		payload := map[string]any{"project_key": "FROMJSON"}
		applyCreateFlags(payload, createFlags{project: "FROMFLAG"})
		if payload["project_key"] != "FROMFLAG" {
			t.Fatalf("project_key = %v, want FROMFLAG", payload["project_key"])
		}
	})

	t.Run("flag labels replace json labels wholesale", func(t *testing.T) {
		payload := map[string]any{"labels": []any{"json-a", "json-b"}}
		applyCreateFlags(payload, createFlags{labels: []string{"flag-only"}})
		if !reflect.DeepEqual(payload["labels"], []string{"flag-only"}) {
			t.Fatalf("labels = %#v, want [flag-only] (replace, not merge)", payload["labels"])
		}
	})
}
