// `issue mine` must accept the same filter flags as `issue list`, except
// --assignee/--reporter: mine pins assignee = currentUser(), so exposing those
// would fight the pin. This pins the shared filter surface end to end through
// the real binary on the credential-free --as-jql preview path: every new
// filter resolves to the same clause it does on `issue list`, the assignee pin
// still applies alongside it, and --assignee/--reporter stay unexposed.
package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestIssueMineSharesListFilterSurface(t *testing.T) {
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

	// Each filter resolves to the same clause `issue list` produces, and the
	// pinned assignee is always present alongside it.
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"priority", []string{"--priority", "High"}, "priority = High"},
		{"single label", []string{"--label", "foo"}, "labels = foo"},
		{"repeated label", []string{"--label", "foo", "--label", "bar"}, "labels in (foo, bar)"},
		{"type", []string{"--type", "Bug"}, "issuetype = Bug"},
		{"epic", []string{"--epic", "KAN-1"}, "parent = KAN-1"},
		{"key", []string{"--key", "KAN-5"}, "key = KAN-5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"issue", "mine", "--as-jql", "--output=json"}, tc.args...)
			got := jqlOf(t, args...)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("mine %v: data.jql missing %q; got %q", tc.args, tc.want, got)
			}
			if !strings.Contains(got, "assignee = currentUser()") {
				t.Fatalf("mine %v dropped the pinned assignee; got %q", tc.args, got)
			}
		})
	}

	// --assignee and --reporter stay unexposed on mine: assignee is pinned, and
	// mine is about the assignee, so reporter has no place here.
	for _, flag := range []string{"--assignee", "--reporter"} {
		out, err := exec.Command(bin, "--config", cfg, "issue", "mine", "--as-jql", flag, "me", "--output=json").CombinedOutput()
		if err == nil {
			t.Fatalf("mine accepted %s; it must stay unexposed:\n%s", flag, out)
		}
		if !strings.Contains(string(out), "unknown flag: "+flag) {
			t.Fatalf("mine %s rejection should name the unknown flag; got:\n%s", flag, out)
		}
	}
}
