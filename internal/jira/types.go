package jira

import (
	"encoding/json"

	"github.com/matcra587/jira-cli/internal/adf"
)

func String(v string) *string { return &v }
func Int(v int) *int          { return &v }
func Bool(v bool) *bool       { return &v }

type Issue struct {
	ID           *string      `json:"id,omitempty"`
	Key          *string      `json:"key,omitempty"`
	Self         *string      `json:"self,omitempty"`
	Fields       *IssueFields `json:"fields,omitempty"`
	Comments     []*Comment   `json:"comments,omitempty"`
	Worklogs     []*Worklog   `json:"worklogs,omitempty"`
	LinkedIssues []*Issue     `json:"linked_issues,omitempty"`
	Subtasks     []*Issue     `json:"subtasks,omitempty"`
}

type IssueFields struct {
	Summary      *string                    `json:"summary,omitempty"`
	IssueType    *IssueType                 `json:"issuetype,omitempty"`
	Description  *adf.Document              `json:"description,omitempty"`
	Status       *Status                    `json:"status,omitempty"`
	Assignee     *User                      `json:"assignee,omitempty"`
	Reporter     *User                      `json:"reporter,omitempty"`
	Priority     *Priority                  `json:"priority,omitempty"`
	Labels       []string                   `json:"labels,omitempty"`
	Components   []Component                `json:"components,omitempty"`
	Updated      *string                    `json:"updated,omitempty"`
	Comment      *CommentPage               `json:"comment,omitempty"`
	Worklog      *WorklogPage               `json:"worklog,omitempty"`
	IssueLinks   []IssueLink                `json:"issuelinks,omitempty"`
	Subtasks     []*Issue                   `json:"subtasks,omitempty"`
	CustomFields map[string]json.RawMessage `json:"-"`
}

type CommentPage struct {
	Comments []*Comment `json:"comments,omitempty"`
}

type WorklogPage struct {
	Worklogs []*Worklog `json:"worklogs,omitempty"`
}

type IssueLink struct {
	InwardIssue  *Issue `json:"inwardIssue,omitempty"`
	OutwardIssue *Issue `json:"outwardIssue,omitempty"`
}

type Status struct {
	Name           *string         `json:"name,omitempty"`
	StatusCategory *StatusCategory `json:"statusCategory,omitempty"`
}

// StatusCategory is the workflow bucket a status belongs to. Key is one of
// "new", "indeterminate", or "done" — stable across projects and used to
// color statuses by category in human output. ColorName is Jira's own color
// designation for the category ("blue-gray", "yellow", "green", "medium-gray"),
// preferred over Key when coloring so the badge matches the Jira UI.
type StatusCategory struct {
	Key       *string `json:"key,omitempty"`
	Name      *string `json:"name,omitempty"`
	ColorName *string `json:"colorName,omitempty"`
}

type Priority struct {
	Name *string `json:"name,omitempty"`
}

// IssueType is the issue's type (Epic, Story, Task, Bug, Sub-task, ...).
type IssueType struct {
	Name    *string `json:"name,omitempty"`
	IconURL *string `json:"iconUrl,omitempty"`
	Subtask *bool   `json:"subtask,omitempty"`
}

type User struct {
	AccountID    *string `json:"accountId,omitempty"`
	AccountType  *string `json:"accountType,omitempty"`
	DisplayName  *string `json:"displayName,omitempty"`
	EmailAddress *string `json:"emailAddress,omitempty"`
	Active       *bool   `json:"active,omitempty"`
	Deleted      *bool   `json:"deleted,omitempty"`
}

type Component struct {
	Name *string `json:"name,omitempty"`
}

type Epic struct {
	Key     *string `json:"key,omitempty"`
	Summary *string `json:"summary,omitempty"`
	Status  *string `json:"status,omitempty"`
}

type SearchResult struct {
	Issues        []*Issue `json:"issues,omitempty"`
	IsLast        bool     `json:"isLast,omitempty"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

type Worklog struct {
	ID               *string       `json:"id,omitempty"`
	TimeSpentSeconds *int          `json:"timeSpentSeconds,omitempty"`
	Started          *string       `json:"started,omitempty"`
	Comment          *adf.Document `json:"comment,omitempty"`
}

type Comment struct {
	ID           *string       `json:"id,omitempty"`
	Self         *string       `json:"self,omitempty"`
	Body         *adf.Document `json:"body,omitempty"`
	Author       *User         `json:"author,omitempty"`
	UpdateAuthor *User         `json:"updateAuthor,omitempty"`
	Created      *string       `json:"created,omitempty"`
	Updated      *string       `json:"updated,omitempty"`
	Visibility   *Visibility   `json:"visibility,omitempty"`
}

// Visibility restricts a comment to a Jira role or a Jira group. Atlassian's
// data model treats Type/Value as mutually exclusive across role and group;
// the CLI's --visibility-role and --visibility-group flags enforce that
// exclusivity locally before any HTTP call ().
type Visibility struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Transition struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

type ProjectFieldSchema struct {
	ProjectKey string        `json:"project_key"`
	IssueType  string        `json:"issue_type"`
	Fields     []FieldSchema `json:"fields,omitempty"`
}

type FieldSchema struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
	// Custom is the schema.custom token Jira reports for a custom
	// field — the trailing identifier of
	// com.atlassian.jira.plugin.system.customfieldtypes:* (e.g.
	// "select", "datepicker", "float"). Empty for system fields and for
	// custom fields whose type Jira does not expose. It is the branch
	// key the mutation pipeline uses to encode a custom-field value.
	Custom string `json:"custom,omitempty"`
}

func (f *IssueFields) UnmarshalJSON(data []byte) error {
	type known IssueFields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var k known
	if err := json.Unmarshal(data, &k); err != nil {
		return err
	}
	*f = IssueFields(k)
	f.CustomFields = map[string]json.RawMessage{}
	for key, val := range raw {
		if len(key) >= len("customfield_") && key[:len("customfield_")] == "customfield_" {
			f.CustomFields[key] = val
		}
	}
	return nil
}

func (f IssueFields) MarshalJSON() ([]byte, error) {
	type known IssueFields
	data, err := json.Marshal(known(f))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	delete(raw, "CustomFields")
	for key, value := range f.CustomFields {
		raw[key] = value
	}
	return json.Marshal(raw)
}

func (i *Issue) UnmarshalJSON(data []byte) error {
	type known Issue
	var k known
	if err := json.Unmarshal(data, &k); err != nil {
		return err
	}
	*i = Issue(k)
	if i.Fields == nil {
		return nil
	}
	if len(i.Comments) == 0 && i.Fields.Comment != nil {
		i.Comments = append([]*Comment(nil), i.Fields.Comment.Comments...)
	}
	if len(i.Worklogs) == 0 && i.Fields.Worklog != nil {
		i.Worklogs = append([]*Worklog(nil), i.Fields.Worklog.Worklogs...)
	}
	if len(i.Subtasks) == 0 {
		i.Subtasks = append([]*Issue(nil), i.Fields.Subtasks...)
	}
	if len(i.LinkedIssues) == 0 {
		for _, link := range i.Fields.IssueLinks {
			if link.OutwardIssue != nil {
				i.LinkedIssues = append(i.LinkedIssues, link.OutwardIssue)
			}
			if link.InwardIssue != nil {
				i.LinkedIssues = append(i.LinkedIssues, link.InwardIssue)
			}
		}
	}
	return nil
}
