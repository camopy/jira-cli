// Nil-safe accessors over the Jira issue model, plus the
// timestamp parsing behind the relative-age rendering.

package issues

import (
	"strings"
	"time"

	"github.com/gechr/x/human"
	"github.com/gechr/x/ptr"

	"github.com/matcra587/jira-cli/internal/jira"
)

// fetchFields are the issue fields the list+sidebar need; requesting a narrow
// set keeps the response small. "description" feeds the sidebar detail pane.
var fetchFields = []string{"summary", "issuetype", "status", "assignee", "reporter", "priority", "labels", "updated", "description"}

func issueKey(i *jira.Issue) string { return ptr.Deref(i.Key) }

func issueSummary(i *jira.Issue) string {
	if i.Fields == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Summary)
}

func issueStatus(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Status == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Status.Name)
}

func issueAssignee(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Assignee == nil {
		return "Unassigned"
	}
	if name := ptr.Deref(i.Fields.Assignee.DisplayName); name != "" {
		return name
	}
	return "Unassigned"
}

func issuePriority(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Priority == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Priority.Name)
}

func issueReporter(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.Reporter == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Reporter.DisplayName)
}

func issueTypeName(i *jira.Issue) string {
	if i.Fields == nil || i.Fields.IssueType == nil {
		return ""
	}
	return ptr.Deref(i.Fields.IssueType.Name)
}

func issueUpdated(i *jira.Issue) string {
	if i.Fields == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Updated)
}

func issueLabels(i *jira.Issue) []string {
	if i.Fields == nil {
		return nil
	}
	return i.Fields.Labels
}

// jiraTimeLayout is the timestamp format Jira's REST API returns. Some
// deployments emit a colon in the zone offset (+05:30) instead, which RFC3339
// covers — parseJiraTime falls back to it.
const jiraTimeLayout = "2006-01-02T15:04:05.000-0700"

// parseJiraTime parses a Jira REST timestamp, falling back to RFC 3339 for
// deployments that emit a colon in the zone offset.
func parseJiraTime(ts string) (time.Time, error) {
	t, err := time.Parse(jiraTimeLayout, ts)
	if err != nil {
		return time.Parse(time.RFC3339, ts)
	}
	return t, nil
}

// age renders a relative age for a Jira timestamp ("now", 5m, 2h, 4d, 3w,
// 6mo, 2y). Empty or unparsable timestamps render "". The thresholds and
// units come from x/human.FormatTimeAgoCompactFrom so the TUI and the
// plain renderers can never disagree on what "3d" means. The library has
// no bare variant, so its " ago" suffix is trimmed here (the list column
// is four cells wide), and clampFuture makes the "in ..." future form
// unreachable rather than trimmed.
func age(ts string, now time.Time) string {
	if ts == "" {
		return ""
	}
	t, err := parseJiraTime(ts)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(human.FormatTimeAgoCompactFrom(clampFuture(t, now), now), " ago")
}

// clampFuture pins a future timestamp to now: clock skew between Jira and
// this machine must read as just-updated ("now"), never as an age or an
// "in ..." forecast.
func clampFuture(t, now time.Time) time.Time {
	if t.After(now) {
		return now
	}
	return t
}

// projectOf returns the project prefix of an issue key ("JCT-12" → "JCT").
func projectOf(key string) string {
	if i := strings.IndexByte(key, '-'); i > 0 {
		return key[:i]
	}
	return ""
}
