package contract

// Contract v2 envelope invariant: data.issue is always an identity object
// carrying at least `key` (cmdutil.IssueRef) — never a bare string — so
// `.data.issue.key` reads identically across every issue-scoped envelope.
// Pinned via --dry-run so every family is exercised without a live Jira;
// decoding into a typed struct is the type pin itself: a regression to a
// bare string fails json.Unmarshal loudly.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMutationEnvelopesCarryIssueRefObject(t *testing.T) {
	attachSrc := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(attachSrc, []byte("attachment body"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"issue.edit", []string{"issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--summary", "renamed"}},
		{"issue.transition", []string{"issue", "transition", "PROJ-1", "--dry-run", "--transition", "31"}},
		{"issue.delete", []string{"issue", "delete", "PROJ-1", "--dry-run", "--no-input"}},
		{"issue.comment.add", []string{"issue", "comment", "add", "PROJ-1", "--dry-run", "--markdown", "hello"}},
		{"issue.comment.delete", []string{"issue", "comment", "delete", "PROJ-1", "10042", "--dry-run"}},
		{"issue.weblink", []string{"issue", "weblink", "PROJ-1", "--dry-run", "--url", "https://example.com"}},
		{"issue.attachment.add", []string{"issue", "attachment", "add", "PROJ-1", "--file", attachSrc, "--dry-run"}},
		{"issue.attachment.delete", []string{"issue", "attachment", "delete", "PROJ-1", "10500", "--dry-run"}},
		{"issue.attachment.download", []string{"issue", "attachment", "download", "PROJ-1", "10500", "--to", "ref-shape-dl.bin", "--dry-run"}},
		{"issue.link.delete", []string{"issue", "link", "delete", "PROJ-1", "9001", "--dry-run"}},
		{"issue.watchers.add", []string{"issue", "watchers", "add", "PROJ-1", "--user", "accountId:712020:abc", "--dry-run"}},
		{"epic.add", []string{"epic", "add", "PROJ-1", "EPIC-1", "--dry-run", "--no-input"}},
		{"epic.remove", []string{"epic", "remove", "PROJ-1", "--dry-run", "--no-input"}},
		{"worklog.add", []string{"worklog", "add", "PROJ-1", "--dry-run", "--time-spent", "1h"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := failOnAnyRequestServer(t)
			cfg := jiraConfig(t, srv.URL)
			args := append([]string{"--config", cfg, "--output=json"}, c.args...)
			stdout, stderr, code := runJira(t, args...)
			if code != 0 {
				t.Fatalf("%s --dry-run exit = %d\nstdout=%s\nstderr=%s", c.name, code, stdout, stderr)
			}
			var env struct {
				Data struct {
					Issue struct {
						Key string `json:"key"`
					} `json:"issue"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("%s envelope does not carry data.issue as an object: %v\nstdout=%s", c.name, err, stdout)
			}
			if env.Data.Issue.Key != "PROJ-1" {
				t.Fatalf("%s data.issue.key = %q, want PROJ-1\nstdout=%s", c.name, env.Data.Issue.Key, stdout)
			}
		})
	}
}
