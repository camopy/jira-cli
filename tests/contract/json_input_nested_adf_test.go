package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// issue edit --json-input must validate nested ADF documents found in the
// fields object before submission. A garbage ADF description inside the
// fields map must fail locally with exit 3, not be forwarded to Jira.
func TestJSONInputNestedADF_IssueEditRejectsUnknownNode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit-bad-adf.json")
	body := `{"fields":{"description":{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x"},
			{"type":"unknown_magic_node"}
		]}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "edit", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json")

	var env struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, stderr, _ := runCommandExpectErrorEnvelope(t, cmd, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("issue edit must reject nested unknown ADF node; got zero errors\n%s", stderr)
	}
	if !containsAny(env.Errors[0].Message, "unknown_magic_node", "unsupported", "unknown") {
		t.Errorf("error must name the unknown node; got: %s", env.Errors[0].Message)
	}
}

// issue edit --json-input with a structurally invalid nested ADF doc
// (heading missing required level attr) must fail before submission.
func TestJSONInputNestedADF_IssueEditRejectsMissingAttr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit-missing-attr.json")
	body := `{"fields":{"description":{"type":"doc","version":1,"content":[
		{"type":"heading","content":[{"type":"text","text":"no level"}]}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "edit", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json")

	var env struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, stderr, _ := runCommandExpectErrorEnvelope(t, cmd, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("issue edit must reject nested ADF heading missing level; got zero errors\n%s", stderr)
	}
}

// issue edit --json-input with a valid nested ADF description must pass.
func TestJSONInputNestedADF_IssueEditAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit-good-adf.json")
	body := `{"fields":{"description":{"type":"doc","version":1,"content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"ok"}]}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "edit", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json")
	stdout, _ := cmd.Output()

	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, stdout)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("valid nested ADF must not error; got %+v\n%s", env.Errors, stdout)
	}
}

// issue comment --json-input with a structurally invalid nested ADF doc
// must fail before submission (heading missing level).
func TestJSONInputNestedADF_CommentRejectsMissingAttr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comment-missing-attr.json")
	body := `{"type":"doc","version":1,"content":[
		{"type":"heading","content":[{"type":"text","text":"no level"}]}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "KAN-1",
		"--json-input", path,
		"--dry-run", "--no-input", "--output=json")

	var env struct {
		Errors []map[string]any `json:"errors"`
	}
	_, stderr, _ := runCommandExpectErrorEnvelope(t, cmd, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("comment must reject nested ADF heading missing level; got zero errors\n%s", stderr)
	}
}
