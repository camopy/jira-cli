package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestOutputSchemasDescribeNestedEnvelopeAndIssueShapes(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "--output=json", "agent", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema error = %v\n%s", err, out)
	}

	var env struct {
		Data struct {
			OutputSchemas map[string]schemaObject `json:"output_schemas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}

	envelope := env.Data.OutputSchemas["envelope"]
	// The envelope is lean: ok, meta, data, errors, warnings.
	for _, required := range []string{"ok", "meta", "data", "errors", "warnings"} {
		if !containsString(envelope.Required, required) {
			t.Fatalf("envelope schema missing required %q: %+v", required, envelope)
		}
	}
	meta := envelope.Properties["meta"]
	// Machine envelopes omit meta.profile entirely; meta requires only
	// command and timestamp.
	for _, required := range []string{"command", "timestamp"} {
		if !containsString(meta.Required, required) {
			t.Fatalf("envelope meta schema missing required %q: %+v", required, meta)
		}
	}
	if containsString(meta.Required, "profile") {
		t.Fatalf("envelope meta schema must not require profile: %+v", meta)
	}
	// The error schema carries the lean structured-error fields.
	errSchema := env.Data.OutputSchemas["error"]
	for _, required := range []string{"type", "code", "message", "hint", "retryable"} {
		if !containsString(errSchema.Required, required) {
			t.Fatalf("error schema missing required %q: %+v", required, errSchema)
		}
	}

	issueList := env.Data.OutputSchemas["issue.list"]
	issues := issueList.Properties["issues"]
	item := issues.Items
	for _, required := range []string{"key", "summary", "status", "updated"} {
		if !containsString(item.Required, required) {
			t.Fatalf("issue.list item schema missing required %q: %+v", required, item)
		}
	}
	// assignee and priority MUST be present in the shape but are nullable per spec.
	if _, ok := item.Properties["assignee"]; !ok {
		t.Fatalf("issue.list item schema missing assignee property: %+v", item)
	}
	if _, ok := item.Properties["priority"]; !ok {
		t.Fatalf("issue.list item schema missing priority property: %+v", item)
	}
	for _, required := range []string{"assignee", "priority"} {
		if containsString(item.Required, required) {
			t.Fatalf("issue.list spec marks %q as nullable; should not be required: %+v", required, item.Required)
		}
	}

	issueEdit := env.Data.OutputSchemas["issue.edit"]
	if !containsString(issueEdit.Required, "fields") {
		t.Fatalf("issue.edit schema missing fields requirement: %+v", issueEdit)
	}
}

type schemaObject struct {
	// Type may be a string ("object") or an array (["object", "null"]) per JSON Schema spec.
	Type       any                     `json:"type"`
	Required   []string                `json:"required"`
	Properties map[string]schemaObject `json:"properties"`
	Items      *schemaObject           `json:"items"`
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
