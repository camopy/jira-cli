// `--count` returns Jira's approximate match count without fetching issues. It
// is wired onto `issue list` and `search jql`. These pin the credential-free
// parts of that wiring through the real binary: the flag is published with the
// right metadata, and it is mutually exclusive with the flags it can't combine
// with (the offline --as-jql preview, and the issue-fetch selectors). The
// count value itself is exercised by the service-level test against a mock.
package contract

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCountFlagMutualExclusions(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	cases := []struct {
		name string
		args []string
		both string
	}{
		{"issue list --as-jql --count", []string{"issue", "list", "--as-jql", "--count"}, "as-jql count"},
		{"search jql --count --web", []string{"search", "jql", "x", "--count", "--web"}, "count web"},
		{"search jql --count --full", []string{"search", "jql", "x", "--count", "--full"}, "count full"},
		{"search jql --count --fields", []string{"search", "jql", "x", "--count", "--fields", "summary"}, "count fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(bin, append([]string{"--config", cfg}, tc.args...)...).CombinedOutput()
			if err == nil {
				t.Fatalf("%s was accepted; the flags must be mutually exclusive:\n%s", tc.name, out)
			}
			if !strings.Contains(string(out), tc.both) {
				t.Fatalf("%s: error should name the conflicting group %q; got:\n%s", tc.name, tc.both, out)
			}
		})
	}
}

// --count lives on `search jql` but NOT on `search saved`: the saved runner
// does not implement count, so exposing the flag there would silently ignore
// it. `search saved --count` must be rejected as an unknown flag.
func TestSearchSavedDoesNotExposeCount(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "search", "saved", "demo", "--count").CombinedOutput()
	if err == nil {
		t.Fatalf("search saved --count was accepted; --count is search jql only:\n%s", out)
	}
	if !strings.Contains(string(out), "unknown flag: --count") {
		t.Fatalf("search saved --count should be an unknown flag; got:\n%s", out)
	}
}

// --count needs a live profile because the estimate comes from Jira (unlike the
// offline --as-jql preview). With no usable client it fails with a validation
// error before any network call, naming the missing profile.
func TestIssueListCountNeedsConfiguredProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "issue", "list", "--count", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("issue list --count without a configured profile was accepted:\n%s", out)
	}
	if !strings.Contains(string(out), "configured profile") {
		t.Fatalf("expected a 'configured profile' validation error; got:\n%s", out)
	}
}
