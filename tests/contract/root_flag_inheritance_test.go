package contract

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// A root persistent flag must be honored identically whether it is
// written before or after the subcommand, and a subcommand must not
// redefine it as a local flag (which would shadow the inherited one and
// split the effective value across two flag objects).
func TestRootNoInputFlagNotShadowedByLocalFlag(t *testing.T) {
	bin := buildJiraBinary(t)

	// `--no-input` placed before the subcommand binds to the root
	// persistent flag; placed after, it must bind to the same flag.
	// With a shadowing local flag the two placements diverge.
	cases := []struct {
		name string
		args []string
	}{
		{"before subcommand", []string{"--no-input", "issue", "create", "--output=json"}},
		{"after subcommand", []string{"issue", "create", "--no-input", "--output=json"}},
	}

	var bodies []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err == nil {
				t.Fatalf("issue create --no-input with no fields succeeded; want a required-field validation error\nstdout=%s", stdout.String())
			}
			body := stderr.String() + stdout.String()
			if !strings.Contains(body, "no-input") {
				t.Fatalf("--no-input %s did not trigger the no-input required-field check:\n%s", tc.name, body)
			}
			bodies = append(bodies, body)
		})
	}
}

// `--no-input` ahead of an `issue attachment delete` (which reads the root
// persistent flag directly) must refuse the destructive op. A subcommand
// that shadows the root flag with a same-name local flag would leave the
// root flag unset and the destructive op would fall through to a prompt.
func TestRootNoInputReachesAttachmentDelete(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--no-input", "issue", "attachment", "delete", "KAN-1", "10001", "--output=json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("attachment delete under --no-input without --force succeeded; want refusal\nstdout=%s", stdout.String())
	}
	body := stderr.String() + stdout.String()
	if !strings.Contains(body, "force") {
		t.Fatalf("attachment delete under --no-input did not require --force:\n%s", body)
	}
}
