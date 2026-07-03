package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

// updateEnvelope is the subset of the failure envelope the update contract
// asserts on.
type updateEnvelope struct {
	OK   bool `json:"ok"`
	Meta struct {
		Command  string `json:"command"`
		ExitCode int    `json:"exit_code"`
	} `json:"meta"`
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

// The contract binary is a plain `go build`: no GoReleaser stamp, no module
// proxy version, not under a Scoop tree — so `jira update` must refuse with
// channel guidance rather than guessing an updater, in every invocation shape.
func TestUpdateUnknownChannelRefusesWithGuidance(t *testing.T) {
	for _, args := range [][]string{
		{"update", "--output=json"},
		{"update", "--dry-run", "--output=json"},
		{"update", "--force", "--output=json"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, _, exitCode := runJira(t, args...)
			if exitCode == 0 {
				t.Fatalf("jira %v: expected non-zero exit, got 0\nstdout=%s", args, stdout)
			}
			var env updateEnvelope
			if err := json.Unmarshal(stdout, &env); err != nil {
				t.Fatalf("jira %v: stdout is not a JSON envelope: %v\nstdout=%s", args, err, stdout)
			}
			if env.OK {
				t.Errorf("jira %v: ok = true, want false", args)
			}
			if env.Meta.Command != "update" {
				t.Errorf("jira %v: meta.command = %q, want %q", args, env.Meta.Command, "update")
			}
			if env.Meta.ExitCode != exitCode {
				t.Errorf("jira %v: meta.exit_code = %d, process exited %d", args, env.Meta.ExitCode, exitCode)
			}
			if len(env.Errors) != 1 {
				t.Fatalf("jira %v: len(errors) = %d, want 1\nstdout=%s", args, len(env.Errors), stdout)
			}
			if env.Errors[0].Type != "validation" {
				t.Errorf("jira %v: errors[0].type = %q, want %q", args, env.Errors[0].Type, "validation")
			}
			if !strings.Contains(env.Errors[0].Message, "install channel") {
				t.Errorf("jira %v: errors[0].message = %q, want install-channel guidance", args, env.Errors[0].Message)
			}
		})
	}
}
