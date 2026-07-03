package cmdutil

import (
	"github.com/matcra587/jira-cli/internal/jira"
)

// DrainTruncationWarnings maps a bounded-drain truncation onto the
// envelope's warnings[], with a resume remediation. nil when the drain
// reached isLast. Shared by every --all consumer of jira.DrainSearch so
// the truncation contract stays identical across commands.
func DrainTruncationWarnings(info jira.DrainInfo) []map[string]any {
	if !info.Truncated {
		return nil
	}
	limit := 100
	if info.TruncatedReason == "max_results" {
		limit = 10_000
	}
	remediation := "Re-run with --unbounded if you need every issue."
	if info.NextPageToken != "" { // pagination-exempt: opaque resume token, no cursor arithmetic
		remediation = "Re-run with --unbounded for everything, or resume from meta.pagination.nextCursor via --cursor."
	}
	return []map[string]any{{
		"type":          "search-truncated",
		"resource":      "issues",
		"reason":        info.TruncatedReason,
		"limit":         limit,
		"pages_fetched": info.PagesFetched,
		"message":       "search truncated by " + info.TruncatedReason + "; re-run with --unbounded to fetch every issue",
		"remediation":   remediation,
	}}
}
