package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The savedquery predictor backs `search saved NAME` completion: it emits one
// line per saved `.jql` query name in sorted order, with the query's
// description (or the JQL body when there is no description) as the
// tab-separated completion hint. Like every emitter it is null-safe and
// sanitized. Black-box: drives the real binary's --@complete hook.
func TestSavedQueryCompletionEmitsQueryNames(t *testing.T) {
	bin := buildJiraBinary(t)

	queriesDir := t.TempDir()
	// One query with frontmatter description, one bare (hint falls back to JQL).
	if err := os.WriteFile(filepath.Join(queriesDir, "open-bugs.jql"),
		[]byte("---\ndescription: Open bugs in KAN\n---\nproject = KAN AND status != Done"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "my-work.jql"),
		[]byte("assignee = currentUser()"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := `default_profile = "default"
queries_path = "` + filepath.ToSlash(queriesDir) + `"

[[profiles]]
name = "default"
base_url = "https://example.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(bin, "--config", cfgPath, "--@complete=savedquery", "--", "").CombinedOutput()
	if err != nil {
		t.Fatalf("complete savedquery: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")

	var gotNames []string
	desc := map[string]string{}
	for _, l := range lines {
		name, hint, _ := strings.Cut(l, "\t")
		gotNames = append(gotNames, name)
		desc[name] = hint
	}

	// Sorted by name for a stable completion order.
	if want := []string{"my-work", "open-bugs"}; !slices.Equal(gotNames, want) {
		t.Fatalf("savedquery names = %q, want %q", gotNames, want)
	}
	if desc["open-bugs"] != "Open bugs in KAN" {
		t.Fatalf("open-bugs hint = %q, want the frontmatter description", desc["open-bugs"])
	}
	// No description: the JQL body is the hint.
	if desc["my-work"] != "assignee = currentUser()" {
		t.Fatalf("my-work hint = %q, want the JQL body", desc["my-work"])
	}
}
