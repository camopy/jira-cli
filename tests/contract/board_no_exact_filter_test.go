// `--exact-filter` is a future opt-in. It MUST NOT ship in this
// feature. This test pins the deferred status by asserting that
// `jira issue list --exact-filter "anything"` exits non-zero with
// cobra's "unknown flag" message.
//
// When the future saved-filter feature ships, this test should be
// updated to use whatever name the future flag adopts (the name is
// provisional today).
package contract

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExactFilterFlagIsNotShipped(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	c := exec.Command(bin, "--config", cfg, "issue", "list", "--exact-filter", "anything")
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for --exact-filter (future opt-in); got success: %s", out)
	}
	combined := string(out)
	// Cobra's standard message is "unknown flag: --exact-filter".
	if !strings.Contains(combined, "unknown flag") || !strings.Contains(combined, "exact-filter") {
		t.Fatalf("expected cobra 'unknown flag: --exact-filter'; got:\n%s", combined)
	}
}
