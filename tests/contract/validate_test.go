// `jql validate` checks JQL through Jira's parser. These pin the credential-free
// wiring through the real binary: the command requires a query, validates the
// --mode value locally, and needs a configured profile (parsing is a Jira call).
// The parse result itself is exercised by the service-level test against a mock.
package contract

import (
	"os/exec"
	"strings"
	"testing"
)

func TestValidateNeedsConfiguredProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "jql", "validate", "project = ENG", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("jql validate without a configured profile was accepted:\n%s", out)
	}
	if !strings.Contains(string(out), "configured profile") {
		t.Fatalf("expected a 'configured profile' validation error; got:\n%s", out)
	}
}

func TestValidateRejectsBadMode(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	// A bad --mode is caught locally before any Jira call.
	out, err := exec.Command(bin, "--config", cfg, "jql", "validate", "project = ENG", "--mode", "bogus", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("jql validate --mode bogus was accepted:\n%s", out)
	}
	if !strings.Contains(string(out), "strict, warn, or none") {
		t.Fatalf("bad --mode should name the valid modes; got:\n%s", out)
	}
}

func TestValidateRequiresAQuery(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "jql", "validate", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("jql validate with no query was accepted:\n%s", out)
	}
}
