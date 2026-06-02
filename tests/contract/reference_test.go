// `jql reference` lists the instance's JQL metadata via /jql/autocompletedata.
// It needs a configured profile (the data comes from Jira); the metadata
// mapping itself is exercised by the service-level test against a mock.
package contract

import (
	"os/exec"
	"strings"
	"testing"
)

func TestJQLReferenceNeedsConfiguredProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "jql", "reference", "--output=json").CombinedOutput()
	if err == nil {
		t.Fatalf("jql reference without a configured profile was accepted:\n%s", out)
	}
	if !strings.Contains(string(out), "configured profile") {
		t.Fatalf("expected a 'configured profile' validation error; got:\n%s", out)
	}
}
