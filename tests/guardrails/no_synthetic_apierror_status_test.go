// MOTIVATION: a hand-built APIError with a LITERAL http status
// (`StatusCode: 404`) fakes a transport failure to borrow its classification —
// so a miss the CLI computes locally (a --type value absent from the fetched
// issuetypes list, a name not in a cached set) surfaces as not-found (exit 2)
// instead of the validation error (exit 3) it really is. A local lookup miss
// must be typed at its source with a dedicated error (e.g.
// jira.IssueTypeUnknownError), never a synthetic 404. Real statuses are copied
// from res.StatusCode by the transport (client.go, attachment.go); this guard
// bans only the numeric-literal form, which is always synthetic.
package guardrails

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Matches a literal numeric status in either form — struct-literal init
// (`StatusCode: 404`) or post-construction assignment (`e.StatusCode = 404`).
// Requiring a digit keeps `StatusCode: res.StatusCode` (a real transport
// status) out of the match.
var syntheticStatusRe = regexp.MustCompile(`StatusCode\s*[:=]\s*\d`)

func TestNoSyntheticAPIErrorStatusInServices(t *testing.T) {
	const dir = "../../internal/jira"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		if syntheticStatusRe.Match(body) {
			offenders = append(offenders, filepath.Join(dir, name))
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("hand-built APIError with a literal http status — a local lookup miss is validation (exit 3), typed at its source, not a synthetic 404:\n%s",
			strings.Join(offenders, "\n"))
	}
}
