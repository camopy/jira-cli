package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The historical field-named Markdown flags survive as hidden deprecated
// aliases of --markdown: existing scripts and agent prompts keep working,
// but no discovery surface advertises them. These tests pin both halves of
// that contract for every command that carries an alias.

func TestMarkdownAliasesStillWork(t *testing.T) {
	tests := map[string][]string{
		"issue edit --description-markdown": {
			"issue", "edit", "PROJ-1", "--dry-run", "--no-input",
			"--description-markdown", "alias body", "--output=json",
		},
		"issue comment add --body-markdown": {
			"issue", "comment", "add", "PROJ-1", "--dry-run", "--no-input",
			"--body-markdown", "alias body", "--output=json",
		},
		"worklog add --comment-markdown": {
			"worklog", "add", "PROJ-1", "--time-spent", "30m", "--dry-run", "--no-input",
			"--comment-markdown", "alias body", "--output=json",
		},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "../../cmd/jira"}, args...)...)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("deprecated alias must keep working, got error = %v\n%s", err, out)
			}
			var env struct {
				OK bool `json:"ok"`
			}
			if jerr := json.Unmarshal(out, &env); jerr != nil {
				t.Fatalf("output not JSON: %v\n%s", jerr, out)
			}
			if !env.OK {
				t.Fatalf("alias dry-run must succeed: %s", out)
			}
		})
	}
}

func TestMarkdownAliasesHiddenFromHelpAndSchema(t *testing.T) {
	helps := map[string][]string{
		"issue edit":        {"issue", "edit", "--help"},
		"issue comment add": {"issue", "comment", "add", "--help"},
		"worklog add":       {"worklog", "add", "--help"},
	}
	for name, args := range helps {
		t.Run(name+" help", func(t *testing.T) {
			out, err := exec.Command("go", append([]string{"run", "../../cmd/jira"}, args...)...).CombinedOutput()
			if err != nil {
				t.Fatalf("--help error = %v\n%s", err, out)
			}
			got := string(out)
			if !strings.Contains(got, "--markdown") {
				t.Fatalf("help must advertise the canonical --markdown flag:\n%s", got)
			}
			for _, alias := range []string{"--body-markdown", "--comment-markdown", "--description-markdown"} {
				if strings.Contains(got, alias) {
					t.Fatalf("help must not advertise the deprecated alias %s:\n%s", alias, got)
				}
			}
		})
	}

	out, err := exec.Command("go", "run", "../../cmd/jira", "agent", "schema", "--output=compact").Output()
	if err != nil {
		t.Fatalf("agent schema error = %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "--markdown") {
		t.Fatalf("schema must carry the canonical --markdown flag")
	}
	for _, alias := range []string{"body-markdown", "comment-markdown", "description-markdown"} {
		if strings.Contains(got, alias) {
			t.Fatalf("schema must not expose the deprecated alias %q", alias)
		}
	}
}
