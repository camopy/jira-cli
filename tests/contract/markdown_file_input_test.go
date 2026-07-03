package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --markdown-file reads the body from a file, sparing multi-paragraph
// Markdown from shell quoting. Same converter, same pipeline, same
// mutual exclusions as inline --markdown.

func TestMarkdownFileReadsBodyFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("## Update\n\nrolled out to **staging**\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"issue", "comment", "add", "PROJ-1", "--dry-run", "--no-input",
		"--markdown-file", path, "--output=json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("comment add --markdown-file error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Comment struct {
				Body map[string]any `json:"body"`
			} `json:"comment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("expected ok envelope: %s", out)
	}
	if got := adfDocText(env.Data.Comment.Body); !strings.Contains(got, "staging") {
		t.Fatalf("file content must reach the converted body, got %q", got)
	}
}

func TestMarkdownFileDashReadsStdin(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira",
		"worklog", "add", "PROJ-1", "--time-spent", "30m", "--dry-run", "--no-input",
		"--markdown-file", "-", "--output=json")
	cmd.Stdin = strings.NewReader("piped **worklog** note\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("worklog add --markdown-file - error = %v\n%s", err, out)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		t.Fatalf("stdin body must convert and succeed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "piped") {
		t.Fatalf("stdin content must reach the worklog comment: %s", out)
	}
}

func TestMarkdownFileExcludesOtherBodySources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for name, extra := range map[string][]string{
		"with --markdown":   {"--markdown", "inline"},
		"with --json-input": {"--json-input", path},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]string{
				"run", "../../cmd/jira",
				"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
				"--markdown-file", path, "--output=json",
			}, extra...)
			out, err := exec.Command("go", args...).CombinedOutput()
			if err == nil {
				t.Fatalf("--markdown-file %s must be rejected:\n%s", name, out)
			}
			if !strings.Contains(string(out), "markdown-file") {
				t.Fatalf("exclusion error must name markdown-file:\n%s", out)
			}
		})
	}
}

// Ticket acceptance for the alias unification: a wire-spelling project in
// the payload satisfies --no-input completeness, and a profile default
// never conflicts with an explicit value it agrees with.
func TestCreateWireProjectSatisfiesCompletenessAndDefaults(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"
secret_backend = "keyring"
default_project = "PROJ"
default_issue_type = "Task"
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	payload := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(payload, []byte(`{"summary":"x","issuetype":{"name":"Task"},"project":{"key":"PROJ"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg,
		"issue", "create", "--no-input", "--dry-run",
		"--json-input", payload, "--output=json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("wire-spelling payload with matching profile default must succeed, got %v\n%s", err, out)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || !env.OK {
		t.Fatalf("expected ok envelope: %s", out)
	}
}
