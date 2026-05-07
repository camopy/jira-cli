package contract

// — ADF wire fidelity contract tests.
//
// These tests verify that:
//   - Wrong-shape ADF (doc.type != "doc", doc.version != 1) is REJECTED
//     with a clear validation error before hitting the wire.
//   - Unknown ADF nodes in strict mode (the mutation-submit default)
//     are REJECTED at the validation stage, not silently forwarded.
//   - The above hold for both the "issue comment" and "issue create" paths.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestI4WrongShapeADFRejected verifies that a valid JSON file that is NOT
// a valid ADF document ({"foo":"bar"}) is rejected before submission with a
// clear shape-violation error in the envelope. Exit code must be 3.
func TestI4WrongShapeADFRejectedByComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrong-shape.json")
	if err := os.WriteFile(path, []byte(`{"foo":"bar"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("expected validation error for wrong-shape ADF; got zero errors\nstdout: %s", stdout)
	}
	msg := env.Errors[0].Message
	if msg == "" {
		t.Fatalf("error message must be non-empty")
	}
	// Must mention 'doc' or 'type' or 'version' to be actionable.
	if !containsAny(msg, "doc", "type", "version") {
		t.Errorf("error message should reference 'doc'/'type'/'version'; got: %s", msg)
	}
}

// TestI4UnknownNodeStrictModeRejectedByComment verifies that an unknown node
// in a comment body is rejected in the default strict mutation mode
// with a validation error naming the unknown node type, not silently forwarded.
func TestI4UnknownNodeStrictModeRejectedByComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown-node.json")
	body := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"prefix"},
			{"type":"unknown_magic_node","attrs":{"x":1}}
		]}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("strict mode must reject unknown ADF node; got zero errors\nstdout: %s", stdout)
	}
	msg := env.Errors[0].Message
	if !containsAny(msg, "unknown_magic_node", "unsupported", "unknown") {
		t.Errorf("error must name the unknown node type; got: %s", msg)
	}
}

// TestI4UnknownNodeBestEffortWarnsNotErrors verifies that best-effort mode
// surfaces a warning for unknown nodes but does NOT abort — the node is
// preserved opaquely.
func TestI4UnknownNodeBestEffortWarnsNotErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown-node.json")
	body := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"prefix"},
			{"type":"unknown_magic_node","attrs":{"x":1}}
		]}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--json",
		"--adf-best-effort")
	stdout, _ := cmd.Output()

	var env struct {
		Data     map[string]any   `json:"data"`
		Errors   []map[string]any `json:"errors"`
		Warnings []struct {
			Message  string `json:"message"`
			NodeType string `json:"node_type"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("best-effort: must NOT error on unknown node; got errors: %+v", env.Errors)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("best-effort: must warn about unknown node; got zero warnings\nstdout: %s", stdout)
	}
	found := false
	for _, w := range env.Warnings {
		if containsAny(w.Message, "unknown_magic_node") || containsAny(w.NodeType, "unknown_magic_node") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning must name 'unknown_magic_node'; warnings: %+v", env.Warnings)
	}
}

// TestI4IllegalMarkOnBlockStrictModeErrors verifies that a mark on a block
// node (paragraph with marks=[strong]) is rejected in strict mode.
func TestI4IllegalMarkOnBlockStrictModeErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-mark.json")
	body := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"strong"}],"content":[
			{"type":"text","text":"x"}
		]}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("strict mode must reject illegal marks on block node; got zero errors\nstdout: %s", stdout)
	}
}

// TestI4UnknownMarkRejectedByStrictMode verifies that an unknown mark type on
// an inline text node is rejected in the default strict mutation mode.
func TestI4UnknownMarkRejectedByStrictMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown-mark.json")
	body := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"hi","marks":[{"type":"unknown_mark","attrs":{"x":1}}]}
		]}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("strict mode must reject unknown ADF mark; got zero errors\nstdout: %s", stdout)
	}
	msg := env.Errors[0].Message
	if !containsAny(msg, "unknown_mark", "unsupported", "unknown") {
		t.Errorf("error must name the unknown mark type; got: %s", msg)
	}
}

// TestI4CreatePathRejectsUnknownAdfNode verifies that issue create --json-input
// with a description containing an unknown node is rejected in strict mode
// (exit 3, envelope.errors[] names the unknown node). .
func TestI4CreatePathRejectsUnknownAdfNode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f007-unknown-node.json")
	body := `{
		"summary": " repro",
		"project_key": "KAN",
		"issue_type": "Task",
		"description": {
			"type": "doc",
			"version": 1,
			"content": [
				{"type": "paragraph", "content": [
					{"type": "text", "text": "x"},
					{"type": "unknown_magic_node"}
				]}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("strict mode must reject unknown ADF node in description; got zero errors\nstdout: %s", stdout)
	}
	msg := env.Errors[0].Message
	if !containsAny(msg, "unknown_magic_node", "unsupported", "unknown") {
		t.Errorf("error must name the unknown node type; got: %s", msg)
	}
}

// TestI4CreatePathRejectsWrongShapeAdf verifies that issue create --json-input
// with a description that claims to be an ADF doc but is malformed (content
// is not an array) is rejected before submission.
func TestI4CreatePathRejectsWrongShapeAdf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f007-wrong-shape.json")
	body := `{"summary": "x", "project_key": "KAN", "issue_type": "Task", "description": {"type": "doc", "version": 1, "content": "not an array"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create",
		"--json-input", path,
		"--dry-run", "--no-input", "--json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("wrong-shape description must be rejected; got zero errors\nstdout: %s", stdout)
	}
	msg := env.Errors[0].Message
	if !containsAny(msg, "doc", "type", "version") {
		t.Errorf("error should reference 'doc'/'type'/'version'; got: %s", msg)
	}
}

// TestI4CreatePathBestEffortPreservesUnknownNode verifies that in best-effort
// mode, issue create with a description containing an unknown node does NOT
// abort — it succeeds (dry-run) with a warning naming the node. .
func TestI4CreatePathBestEffortPreservesUnknownNode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f007-best-effort.json")
	body := `{
		"summary": " best-effort",
		"project_key": "KAN",
		"issue_type": "Task",
		"description": {
			"type": "doc",
			"version": 1,
			"content": [
				{"type": "paragraph", "content": [
					{"type": "text", "text": "x"},
					{"type": "unknown_magic_node"}
				]}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "create",
		"--json-input", path,
		"--dry-run", "--no-input", "--json",
		"--adf-best-effort")
	stdout, _ := cmd.Output()

	var env struct {
		Data     map[string]any   `json:"data"`
		Errors   []map[string]any `json:"errors"`
		Warnings []struct {
			Message  string `json:"message"`
			NodeType string `json:"node_type"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output is not parseable JSON: %v\nstdout: %s", err, stdout)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("best-effort: must NOT error on unknown node; got errors: %+v\nstdout: %s", env.Errors, stdout)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("best-effort: must warn about unknown node; got zero warnings\nstdout: %s", stdout)
	}
	found := false
	for _, w := range env.Warnings {
		if containsAny(w.Message, "unknown_magic_node") || containsAny(w.NodeType, "unknown_magic_node") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning must name 'unknown_magic_node'; warnings: %+v", env.Warnings)
	}
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 {
			if len(s) >= len(sub) {
				for i := 0; i <= len(s)-len(sub); i++ {
					if s[i:i+len(sub)] == sub {
						return true
					}
				}
			}
		}
	}
	return false
}
