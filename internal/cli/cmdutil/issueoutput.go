package cmdutil

import (
	"encoding/json"

	"github.com/matcra587/jira-cli/internal/jira"
)

// IssueOutput renders a list of issues for an envelope's data payload. With
// detail it returns the full issue records; otherwise it returns the compact
// per-issue summary maps. Shared by the issue list and search commands so
// both surface an identical compact shape.
func IssueOutput(issues []*jira.Issue, detail bool) any {
	if detail {
		return issues
	}
	out := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		out = append(out, IssueSummary(issue))
	}
	return out
}

// issueSummaryKeys maps a Jira field id onto the summary keys it populates.
// status fans out to its category companions so a narrowed projection never
// renames or re-types the keys the default summary publishes.
var issueSummaryKeys = map[string][]string{
	"key":      {"key"},
	"summary":  {"summary"},
	"status":   {"status", "status_category", "status_color"},
	"assignee": {"assignee"},
	"priority": {"priority"},
	"updated":  {"updated"},
}

// IssueOutputFields projects issues onto the compact summary shape narrowed
// to the requested fields, so a field selector trims the default projection
// instead of switching to Jira's raw wire shape. Summary fields keep their
// summary keys and types; any other requested field rides top-level under its
// Jira id with the wire value, or null when Jira omitted it. key is always
// present as the row identity.
func IssueOutputFields(issues []*jira.Issue, fields []string) []map[string]any {
	out := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issueSummaryFields(issue, fields))
	}
	return out
}

func issueSummaryFields(issue *jira.Issue, fields []string) map[string]any {
	full := IssueSummary(issue)
	row := map[string]any{"key": full["key"]}
	var wire map[string]any // decoded fields block, for non-summary requests
	for _, field := range fields {
		keys, ok := issueSummaryKeys[field]
		if !ok {
			if wire == nil {
				wire = issueWireFields(issue)
			}
			row[field] = wire[field]
			continue
		}
		for _, key := range keys {
			// status_color has no placeholder — it appears only when
			// Jira reports it, matching the default summary.
			if v, ok := full[key]; ok {
				row[key] = v
			}
		}
	}
	return row
}

// issueWireFields decodes the issue's fields block back into a generic map so
// a projection can carry non-summary fields (issuetype, labels,
// customfield_*) under their wire ids.
func issueWireFields(issue *jira.Issue) map[string]any {
	if issue == nil || issue.Fields == nil {
		return nil
	}
	data, err := json.Marshal(issue.Fields)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// IssueSummary projects an issue onto the stable compact field set
// (key, summary, status, assignee, priority, updated). Null-safe: missing
// fields keep their zero placeholder so the shape never varies.
func IssueSummary(issue *jira.Issue) map[string]any {
	summary := map[string]any{
		"key":             "",
		"summary":         "",
		"status":          "",
		"status_category": "",
		"assignee":        nil,
		"priority":        nil,
		"updated":         "",
	}
	if issue == nil {
		return summary
	}
	if issue.Key != nil {
		summary["key"] = *issue.Key
	}
	if issue.Fields == nil {
		return summary
	}
	if issue.Fields.Summary != nil {
		summary["summary"] = *issue.Fields.Summary
	}
	if status := issue.Fields.Status; status != nil {
		if status.Name != nil {
			summary["status"] = *status.Name
		}
		if status.StatusCategory != nil && status.StatusCategory.Key != nil {
			summary["status_category"] = *status.StatusCategory.Key
		}
		if status.StatusCategory != nil && status.StatusCategory.ColorName != nil {
			summary["status_color"] = *status.StatusCategory.ColorName
		}
	}
	if user := issue.Fields.Assignee; user != nil {
		summary["assignee"] = AssigneeSummary(user)
	}
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != nil {
		summary["priority"] = *issue.Fields.Priority.Name
	}
	if issue.Fields.Updated != nil {
		summary["updated"] = *issue.Fields.Updated
	}
	return summary
}

// AssigneeSummary projects a user onto the compact {account_id, display_name}
// shape used inside an issue summary.
func AssigneeSummary(user *jira.User) map[string]any {
	out := map[string]any{
		"account_id":   "",
		"display_name": "",
	}
	if user.AccountID != nil {
		out["account_id"] = *user.AccountID
	}
	if user.DisplayName != nil {
		out["display_name"] = *user.DisplayName
	}
	return out
}
