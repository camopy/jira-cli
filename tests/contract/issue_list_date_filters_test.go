// The --updated/--created/--resolved timeframe flags must reach the JQL
// builder from every surface that exposes them — `issue list`, `issue mine`,
// and `jql build` — and compile to the documented date clauses. This pins the
// flag-to-builder wiring end to end through the real binary, on the
// credential-free `--as-jql` preview path so no Jira call is needed.
package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestIssueListDateFiltersWorkWithoutCredentials(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "issue list relative lower bound",
			args: []string{"issue", "list", "--as-jql", "--updated", "-7d", "--output=json"},
			want: "updated >= -7d",
		},
		{
			name: "issue list absolute range is quoted",
			args: []string{"issue", "list", "--as-jql", "--created", "2026-01-01..2026-02-01", "--output=json"},
			want: `created >= "2026-01-01" AND created <= "2026-02-01"`,
		},
		{
			name: "issue mine comparator alongside currentUser",
			args: []string{"issue", "mine", "--as-jql", "--resolved", "<=2026-02-01", "--output=json"},
			want: `resolved <= "2026-02-01"`,
		},
		{
			name: "jql build relative with comparator",
			args: []string{"jql", "build", "--updated", ">=-2w", "--output=json"},
			want: "updated >= -2w",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(bin, append([]string{"--config", cfg}, tc.args...)...).CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			// Assert on the decoded data.jql, not raw bytes: the JSON encoder
			// escapes >, < and " (e.g. >= becomes >=), so a raw substring
			// match on the comparator clauses would never hit.
			var env struct {
				Data struct {
					JQL string `json:"jql"`
				} `json:"data"`
			}
			if jsonErr := json.Unmarshal(out, &env); jsonErr != nil {
				t.Fatalf("parse envelope: %v\n%s", jsonErr, out)
			}
			if !strings.Contains(env.Data.JQL, tc.want) {
				t.Fatalf("data.jql missing %q; got %q", tc.want, env.Data.JQL)
			}
		})
	}

	// An unsigned relative duration is rejected before any Jira call, with a
	// message that names the offending value. The diagnostic is plain text on
	// stderr, so a raw substring match is correct here.
	out, err := exec.Command(bin, "--config", cfg, "issue", "list", "--as-jql", "--updated", "7d", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("unsigned relative --updated 7d was accepted:\n%s", out)
	}
	if !strings.Contains(string(out), "signed relative duration") {
		t.Fatalf("rejection message missing guidance; got:\n%s", out)
	}
}

// `issue mine` carries a reduced manual flag set; it now shares the sort flags
// so a date-filtered "mine" query can also be ordered. It previously rejected
// --order-by outright, and its default sort is descending like `issue list`.
func TestIssueMineAcceptsSortFlags(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	jqlOf := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := exec.Command(bin, append([]string{"--config", cfg}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		var env struct {
			Data struct {
				JQL string `json:"jql"`
			} `json:"data"`
		}
		if jsonErr := json.Unmarshal(out, &env); jsonErr != nil {
			t.Fatalf("parse envelope: %v\n%s", jsonErr, out)
		}
		return env.Data.JQL
	}

	if got := jqlOf(t, "issue", "mine", "--order-by", "created", "--as-jql", "--output=json"); !strings.Contains(got, "ORDER BY created DESC") {
		t.Fatalf("mine --order-by created not applied; got %q", got)
	}
	if got := jqlOf(t, "issue", "mine", "--order-by", "created", "--desc=false", "--as-jql", "--output=json"); !strings.Contains(got, "ORDER BY created ASC") {
		t.Fatalf("mine --desc=false not applied; got %q", got)
	}
	if got := jqlOf(t, "issue", "mine", "--as-jql", "--output=json"); !strings.Contains(got, "ORDER BY updated DESC") {
		t.Fatalf("mine default sort should be descending; got %q", got)
	}
}
