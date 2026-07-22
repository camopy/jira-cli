package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// worklog add --json-input must accept the canonical ADF `comment`
// field — a raw ADF document — the same shape comment/create accept,
// not only the `comment_markdown` string form.
func TestADFInputShape_WorklogAcceptsCanonicalComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worklog-comment-adf.json")
	body := `{"time_spent":"30m","comment":{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"text","text":"paired"}]}
	]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t),
		"--output=json", "worklog", "add", "PROJ-1",
		"--json-input", path, "--dry-run", "--no-input")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add error = %v\n%s", err, out)
	}
	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output not JSON: %v\n%s", err, out)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("canonical ADF comment must be accepted; got %+v\n%s", env.Errors, out)
	}
}

// worklog add --json-input with an invalid nested ADF comment doc must
// fail before submission — the canonical comment field is validated
// just like comment/create ADF input.
func TestADFInputShape_WorklogRejectsInvalidComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worklog-bad-comment-adf.json")
	body := `{"time_spent":"30m","comment":{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x"},
			{"type":"unknown_magic_node"}
		]}
	]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t),
		"--output=json", "worklog", "add", "PROJ-1",
		"--json-input", path, "--dry-run", "--no-input")
	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	_, _, _ = runCommandExpectErrorEnvelope(t, cmd, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("invalid ADF comment must be rejected; got zero errors")
	}
}

// agent schema must publish the canonical ADF input shape so dry-run,
// live submit, and the schema surface describe the same fields. Docent
// inlines the shared adf_document shape separately at every --json-input
// leaf that accepts a rich-text body, so each inlining site is checked —
// one drifting leaf must fail, not hide behind the others.
func TestADFInputShape_AgentSchemaDescribesCanonicalShape(t *testing.T) {
	root := loadAgentSchema(t)
	// path → property chain from the input schema root to the ADF
	// document node ("" means the input schema itself is the document).
	for path, prop := range map[string]string{
		"jira issue comment":      "",
		"jira issue comment add":  "",
		"jira issue comment edit": "",
		"jira issue create":       "description",
		"jira issue transition":   "comment",
		"jira worklog add":        "comment",
	} {
		cmd := findSchemaCommand(root, path)
		if cmd == nil {
			t.Fatalf("agent schema missing path %q", path)
		}
		doc := cmd.InputSchema
		if prop != "" {
			props, _ := doc["properties"].(map[string]any)
			doc, _ = props[prop].(map[string]any)
		}
		props, _ := doc["properties"].(map[string]any)
		typeProp, _ := props["type"].(map[string]any)
		enum, _ := typeProp["enum"].([]any)
		if len(enum) != 1 || enum[0] != "doc" {
			t.Errorf("%s: ADF document shape not inlined at %q: %v", path, prop, doc)
		}
	}
}
