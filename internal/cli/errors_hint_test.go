package cli

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
	"github.com/matcra587/jira-cli/internal/jira"
)

// TestEveryMappedJiraStatusCarriesAHint pins the invariant that every
// HTTP status jira.CodeForStatus resolves to a stable jira_* code must
// also carry a non-empty registry hint. A mapped code with an empty hint
// hands the caller a failure with no next step — exactly the gap that
// 400/409/410 silently had. This guard fails the moment a new status
// mapping is added without a matching registry row.
func TestEveryMappedJiraStatusCarriesAHint(t *testing.T) {
	t.Parallel()
	statuses := []int{400, 401, 403, 404, 409, 410, 429, 500, 502, 503}
	for _, status := range statuses {
		code := jira.CodeForStatus(status)
		if !strings.HasPrefix(string(code), "jira_") {
			t.Errorf("status %d resolves to %q, not a stable jira_* code", status, code)
		}
		spec, ok := errtax.Lookup(code)
		if !ok || spec.Hint == "" {
			t.Errorf("status %d (code %q) has no registry hint — every mapped Jira status must carry remediation", status, code)
		}
	}
}
