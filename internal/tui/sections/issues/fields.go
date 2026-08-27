// Nil-safe accessors over the Jira issue model, plus the
// timestamp parsing behind the relative-age rendering.

package issues

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	termansi "github.com/gechr/x/ansi"
	"github.com/gechr/x/human"
	"github.com/gechr/x/ptr"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// fetchFields are the issue fields the list+sidebar need; requesting a narrow
// set keeps the response small. "description" feeds the sidebar detail pane.
var fetchFields = []string{"summary", "issuetype", "status", "assignee", "reporter", "priority", "labels", "updated", "description"}

func issueFetchFields(custom []core.CustomField) []string {
	return append(append([]string(nil), fetchFields...), customFieldIDs(custom)...)
}

func customFieldIDs(custom []core.CustomField) []string {
	ids := make([]string, len(custom))
	for i, field := range custom {
		ids[i] = field.ID
	}
	return ids
}

func customFieldValue(i *jira.Issue, field core.CustomField) string {
	if i == nil || i.Fields == nil {
		return "—"
	}
	raw, ok := i.Fields.CustomFields[field.ID]
	if !ok {
		return "—"
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "—"
	}
	return truncCells(formatCustomFieldValue(value), 200)
}

func formatCustomFieldValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "—"
	case string:
		return sanitizeCustomFieldText(v)
	case json.Number:
		return v.String()
	case bool:
		return fmt.Sprint(v)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = formatCustomFieldValue(item)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, key := range []string{"displayName", "name", "value", "key"} {
			if item, ok := v[key]; ok {
				return formatCustomFieldValue(item)
			}
		}
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
	}
	return fmt.Sprint(value)
}

func customFieldName(field core.CustomField) string {
	return customFieldColumnLabel(field)
}

func customFieldColumnLabel(field core.CustomField) string {
	return sanitizeCustomFieldText(field.ColumnLabel())
}

func sanitizeCustomFieldText(value string) string {
	value = termansi.Strip(value)
	value = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func issueKey(i *jira.Issue) string { return ptr.Deref(i.Key) }

func issueSummary(i *jira.Issue) string {
	if i.Fields == nil {
		return ""
	}
	return ptr.Deref(i.Fields.Summary)
}

// issueStatusCategory returns the status's category key and Jira color name
// ("", "" when the fetch did not expand them), the inputs the shared pill
// palette keys on.
func issueStatusCategory(i *jira.Issue) (category, colorName string) {
	if i == nil || i.Fields == nil || i.Fields.Status == nil || i.Fields.Status.StatusCategory == nil {
		return "", ""
	}
	sc := i.Fields.Status.StatusCategory
	return ptr.Deref(sc.Key), ptr.Deref(sc.ColorName)
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
