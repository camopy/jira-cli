package cmdutil

import "github.com/matcra587/jira-cli/internal/jira"

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
