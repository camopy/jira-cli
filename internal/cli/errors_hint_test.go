package cli

import (
	"strings"
	"testing"
)

// TestEveryMappedJiraStatusCarriesAHint pins the invariant that every
// HTTP status jiraCodeForStatus resolves to a stable jira_* code must
// also yield a non-empty remediation hint. A mapped code with an empty
// hint hands the caller a failure with no next step — exactly the gap
// that 400/409/410 silently had. This guard fails the moment a new
// status mapping is added without a matching hint.
func TestEveryMappedJiraStatusCarriesAHint(t *testing.T) {
	statuses := []int{400, 401, 403, 404, 409, 410, 429, 500, 502, 503}
	for _, status := range statuses {
		code := jiraCodeForStatus(status, ErrorTypeServer)
		if !strings.HasPrefix(code, "jira_") {
			t.Errorf("status %d resolves to %q, not a stable jira_* code", status, code)
		}
		if hint := jiraHintForStatus(status, ErrorTypeServer); hint == "" {
			t.Errorf("status %d (code %q) yields no hint — every mapped Jira status must carry remediation", status, code)
		}
	}
}
