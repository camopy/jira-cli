package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueKeyParallelHelpSurfaceCoversEveryBulkCommand(t *testing.T) {
	bin := buildJiraBinary(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "issue view", args: []string{"issue", "view"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue list key filter", args: []string{"issue", "list"}, want: []string{"--key", "--parallelism"}},
		{name: "issue edit", args: []string{"issue", "edit"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue clone", args: []string{"issue", "clone"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue move", args: []string{"issue", "move"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue delete", args: []string{"issue", "delete"}, want: []string{"KEY", "--parallelism", "--force"}},
		{name: "issue attachment add", args: []string{"issue", "attachment", "add"}, want: []string{"KEY", "--file", "--parallelism"}},
		{name: "issue attachment list", args: []string{"issue", "attachment", "list"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue comment alias", args: []string{"issue", "comment"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue comment add", args: []string{"issue", "comment", "add"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue comment list", args: []string{"issue", "comment", "list"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue link create", args: []string{"issue", "link"}, want: []string{"KEY", "--to", "--type", "--parallelism"}},
		{name: "issue link list", args: []string{"issue", "link", "list"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue weblink", args: []string{"issue", "weblink"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue watch", args: []string{"issue", "watch"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue unwatch", args: []string{"issue", "unwatch"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue watchers add", args: []string{"issue", "watchers", "add"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue watchers remove", args: []string{"issue", "watchers", "remove"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue watchers list", args: []string{"issue", "watchers", "list"}, want: []string{"KEY", "--parallelism"}},
		{name: "issue transition", args: []string{"issue", "transition"}, want: []string{"KEY", "--parallelism"}},
		{name: "worklog add", args: []string{"worklog", "add"}, want: []string{"KEY", "--parallelism"}},
		{name: "worklog list", args: []string{"worklog", "list"}, want: []string{"KEY", "--parallelism"}},
		{name: "epic add", args: []string{"epic", "add"}, want: []string{"ISSUE_KEY", "EPIC_KEY", "--parallelism"}},
		{name: "epic remove", args: []string{"epic", "remove"}, want: []string{"ISSUE_KEY", "--parallelism"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := exec.Command(bin, append(tt.args, "--help")...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s --help error = %v\n%s", tt.name, err, out)
			}
			got := string(out)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("%s --help missing %q\n%s", tt.name, want, got)
				}
			}
		})
	}
}

func TestSecondaryIDCommandsRemainSingleTarget(t *testing.T) {
	bin := buildJiraBinary(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "comment edit", args: []string{"issue", "comment", "edit"}, want: []string{"KEY", "COMMENT_ID"}},
		{name: "comment delete", args: []string{"issue", "comment", "delete"}, want: []string{"KEY", "COMMENT_ID"}},
		{name: "attachment download", args: []string{"issue", "attachment", "download"}, want: []string{"KEY", "ATTACHMENT_ID"}},
		{name: "attachment delete", args: []string{"issue", "attachment", "delete"}, want: []string{"KEY", "ATTACHMENT_ID"}},
		{name: "link delete", args: []string{"issue", "link", "delete"}, want: []string{"KEY", "LINK_ID"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := exec.Command(bin, append(tt.args, "--help")...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s --help error = %v\n%s", tt.name, err, out)
			}
			got := string(out)
			if strings.Contains(got, "--parallelism") {
				t.Fatalf("%s unexpectedly exposes --parallelism\n%s", tt.name, got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("%s --help missing %q\n%s", tt.name, want, got)
				}
			}
		})
	}
}

func TestBulkMutationDryRunsAcceptRangesWithoutCredentials(t *testing.T) {
	bin := buildJiraBinary(t)
	attachmentPath := filepath.Join(t.TempDir(), "bulk.txt")
	if err := os.WriteFile(attachmentPath, []byte("bulk attachment"), 0o600); err != nil {
		t.Fatalf("WriteFile(attachment) error = %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "comment add", args: []string{"issue", "comment", "add", "PROJ-1..2", "--body-markdown", "bulk", "--dry-run"}},
		{name: "legacy comment alias", args: []string{"issue", "comment", "PROJ-1..2", "--body-markdown", "bulk", "--dry-run"}},
		{name: "worklog add", args: []string{"worklog", "add", "PROJ-1..2", "--time-spent", "15m", "--comment-markdown", "bulk", "--dry-run"}},
		{name: "watchers add", args: []string{"issue", "watchers", "add", "PROJ-1..2", "--user", "accountId:bulk-user", "--dry-run"}},
		{name: "watchers remove", args: []string{"issue", "watchers", "remove", "PROJ-1..2", "--user", "accountId:bulk-user", "--dry-run"}},
		{name: "epic add", args: []string{"epic", "add", "PROJ-1..2", "EPIC-1", "--dry-run"}},
		{name: "epic remove", args: []string{"epic", "remove", "PROJ-1..2", "--dry-run"}},
		{name: "attachment add", args: []string{"issue", "attachment", "add", "PROJ-1..2", "--file", attachmentPath, "--dry-run"}},
		{name: "issue link create", args: []string{"issue", "link", "PROJ-1..2", "--to", "PROJ-99", "--type", "Blocks", "--dry-run"}},
		{name: "issue weblink", args: []string{"issue", "weblink", "PROJ-1..2", "--url", "https://example.com/bulk", "--title", "Bulk", "--dry-run"}},
		{name: "issue edit", args: []string{"issue", "edit", "PROJ-1..2", "--summary", "Bulk edit", "--dry-run"}},
		{name: "issue transition execute", args: []string{"issue", "transition", "PROJ-1..2", "--transition", "21", "--dry-run"}},
		{name: "issue clone", args: []string{"issue", "clone", "PROJ-1..2", "--dry-run"}},
		{name: "issue move", args: []string{"issue", "move", "PROJ-1..2", "--dry-run"}},
		{name: "issue delete", args: []string{"issue", "delete", "PROJ-1..2", "--dry-run"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, append([]string{"--output=json"}, tt.args...)...)
			cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s dry-run error = %v\nstdout=%s\nstderr=%s", tt.name, err, stdout.String(), stderr.String())
			}

			var env struct {
				OK   bool `json:"ok"`
				Data struct {
					Succeeded int `json:"succeeded"`
					Failed    int `json:"failed"`
					Results   []struct {
						Key  string          `json:"key"`
						OK   bool            `json:"ok"`
						Data json.RawMessage `json:"data"`
					} `json:"results"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("%s stdout envelope is not JSON: %v\nstdout=%s\nstderr=%s", tt.name, err, stdout.String(), stderr.String())
			}
			if !env.OK || env.Data.Succeeded != 2 || env.Data.Failed != 0 || len(env.Data.Results) != 2 {
				t.Fatalf("%s dry-run summary = ok %v succeeded %d failed %d results %d\n%s",
					tt.name, env.OK, env.Data.Succeeded, env.Data.Failed, len(env.Data.Results), stdout.String())
			}
			if env.Data.Results[0].Key != "PROJ-1" || env.Data.Results[1].Key != "PROJ-2" ||
				!env.Data.Results[0].OK || !env.Data.Results[1].OK ||
				len(env.Data.Results[0].Data) == 0 || len(env.Data.Results[1].Data) == 0 {
				t.Fatalf("%s dry-run results = %+v", tt.name, env.Data.Results)
			}
		})
	}
}

func TestMultiKeyDestructiveCommandsRequireForceForLiveWrites(t *testing.T) {
	bin := buildJiraBinary(t)
	for _, sub := range []string{"clone", "move", "delete"} {
		t.Run(sub, func(t *testing.T) {
			cmd := exec.Command(bin, "--output=json", "issue", sub, "PROJ-1..2")
			var env struct {
				OK     bool `json:"ok"`
				Errors []struct {
					Type    string `json:"type"`
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"errors"`
			}
			_, _, err := runCommandExpectErrorEnvelope(t, cmd, &env)
			assertValidationExitCode(t, err)
			if env.OK || len(env.Errors) == 0 {
				t.Fatalf("issue %s force error envelope = %+v", sub, env)
			}
			if !strings.Contains(env.Errors[0].Message, "multiple keys requires --force") {
				t.Fatalf("issue %s force error message = %q, want multi-key force requirement", sub, env.Errors[0].Message)
			}
		})
	}
}
