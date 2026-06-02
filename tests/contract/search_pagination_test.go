// `search jql` gains --all/--limit/--unbounded for bounded token pagination.
// These pin the credential-free wiring: the flags are mutually exclusive with
// the no-fetch modes (--count, --web), and they live only on `search jql`, not
// `search saved` (whose runner does not implement them). The drain logic itself
// is exercised by the service-level DrainSearch tests against a mock.
package contract

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSearchPaginationMutualExclusions(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	cases := []struct {
		args []string
		both string
	}{
		{[]string{"search", "jql", "x", "--count", "--all"}, "count all"},
		{[]string{"search", "jql", "x", "--count", "--limit", "10"}, "count limit"},
		{[]string{"search", "jql", "x", "--web", "--all"}, "web all"},
		{[]string{"search", "jql", "x", "--web", "--limit", "10"}, "web limit"},
	}
	for _, tc := range cases {
		out, err := exec.Command(bin, append([]string{"--config", cfg}, tc.args...)...).CombinedOutput()
		if err == nil {
			t.Fatalf("%v was accepted; the flags must be mutually exclusive:\n%s", tc.args, out)
		}
		if !strings.Contains(string(out), tc.both) {
			t.Fatalf("%v: error should name the conflicting group %q; got:\n%s", tc.args, tc.both, out)
		}
	}
}

// --all/--limit/--unbounded are search-jql-only; `search saved` must reject them.
func TestSearchSavedDoesNotExposePaginationFlags(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	for _, flag := range []string{"--all", "--limit", "--unbounded"} {
		args := []string{"--config", cfg, "search", "saved", "demo", flag}
		if flag == "--limit" {
			args = append(args, "10")
		}
		out, err := exec.Command(bin, args...).CombinedOutput()
		if err == nil {
			t.Fatalf("search saved %s was accepted; it is search-jql only:\n%s", flag, out)
		}
		if !strings.Contains(string(out), "unknown flag: "+flag) {
			t.Fatalf("search saved %s should be an unknown flag; got:\n%s", flag, out)
		}
	}
}
