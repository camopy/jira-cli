package startup_test

import (
	"reflect"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/startup"
)

func TestGlobalsFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want startup.Globals
	}{
		{"none", []string{"issue", "list"}, startup.Globals{}},
		{"long config space", []string{"--config", "/x", "issue"}, startup.Globals{ConfigPath: "/x"}},
		{"long config equals", []string{"--config=/x"}, startup.Globals{ConfigPath: "/x"}},
		{"short config space", []string{"-c", "/x"}, startup.Globals{ConfigPath: "/x"}},
		{"short config glued", []string{"-c/x"}, startup.Globals{ConfigPath: "/x"}},
		{"long profile space", []string{"--profile", "work"}, startup.Globals{Profile: "work"}},
		{"short profile glued", []string{"-Pwork"}, startup.Globals{Profile: "work"}},
		{"both", []string{"-P", "work", "--config", "/y", "issue"}, startup.Globals{ConfigPath: "/y", Profile: "work"}},
		{"output value not mistaken for config or profile", []string{"-o", "json", "-c", "/x"}, startup.Globals{ConfigPath: "/x"}},
		{"lowercase p no longer selects profile", []string{"-p", "work"}, startup.Globals{}},
		{"terminator stops scan", []string{"--", "--config", "/x"}, startup.Globals{}},
		{"trailing valueless config", []string{"--config"}, startup.Globals{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startup.GlobalsFromArgs(tc.args); got != tc.want {
				t.Fatalf("GlobalsFromArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestSplitFirstCommandArg(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantPrefix  []string
		wantCommand string
		wantRest    []string
		wantOK      bool
	}{
		{"command after global flag", []string{"--config", "/x", "issue", "list"}, []string{"--config", "/x"}, "issue", []string{"list"}, true},
		{"bare command", []string{"issue", "list"}, []string{}, "issue", []string{"list"}, true},
		{"valued global flag before command", []string{"--timeout", "5", "issue", "list"}, []string{"--timeout", "5"}, "issue", []string{"list"}, true},
		{"short output before command", []string{"-o", "json", "issue", "list"}, []string{"-o", "json"}, "issue", []string{"list"}, true},
		{"combined short output before command", []string{"-ojson", "issue", "list"}, []string{"-ojson"}, "issue", []string{"list"}, true},
		{"long output before command", []string{"--output", "json", "issue", "list"}, []string{"--output", "json"}, "issue", []string{"list"}, true},
		{"only flags no command", []string{"--config", "/x"}, []string{"--config", "/x"}, "", nil, false},
		{"double dash escapes command", []string{"--", "cmd"}, []string{"--"}, "cmd", []string{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, command, rest, ok := startup.SplitFirstCommandArg(tc.args)
			if command != tc.wantCommand || ok != tc.wantOK ||
				!reflect.DeepEqual(prefix, tc.wantPrefix) || !reflect.DeepEqual(rest, tc.wantRest) {
				t.Fatalf("SplitFirstCommandArg(%v) = (%v, %q, %v, %v), want (%v, %q, %v, %v)",
					tc.args, prefix, command, rest, ok, tc.wantPrefix, tc.wantCommand, tc.wantRest, tc.wantOK)
			}
		})
	}
}
