package envelope

import (
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Typed Output structs for the work/agile/search/user operation family —
// worklog, epic, boards, search, jql, me, and user.search. Each struct is the
// single declaration of its envelope's `data` shape: the builder emits it,
// SchemaOf derives the published schema from it, and the registration beside
// it feeds the exhaustiveness guardrail. Registration uses a package-level var
// (not init) so the wiring is explicit and greppable.

// WorklogDraft is the parsed-but-unsubmitted worklog `worklog add --dry-run`
// echoes back: the values that would be sent to Jira, with no server-assigned
// id. The live add carries a *jira.Worklog in the same place instead, so the
// carrier field (WorklogAddOutput.Worklog) is typed any.
type WorklogDraft struct {
	TimeSpentSeconds int           `json:"timeSpentSeconds"`
	Started          string        `json:"started"`
	Comment          *adf.Document `json:"comment"`
}

// WorklogAddOutput is `worklog add`'s envelope data. Worklog holds two shapes
// by path: a *jira.Worklog on the live add (Jira's native worklog, with its
// server id) and a *WorklogDraft under --dry-run (the parsed submission, no
// id). One any field carries both because the shapes genuinely differ — the
// live record is omitempty-sparse, the draft is always-present — so neither
// can stand in for the other without changing emitted bytes. The doc override
// on registration restores the object sub-schema the any erases.
type WorklogAddOutput struct {
	Issue   IssueRef `json:"issue"`
	Worklog any      `json:"worklog"`
	DryRun  bool     `json:"dry_run"`
}

var _ = register("worklog.add", WorklogAddOutput{}, map[string]any{
	"properties": map[string]any{
		"worklog": map[string]any{
			"type":        "object",
			"description": "The worklog: Jira's native record on a live add (with its server id), or the parsed submission echoed under --dry-run (no id).",
			"properties": map[string]any{
				"id":               map[string]any{"type": "string"},
				"timeSpentSeconds": map[string]any{"type": "integer"},
				"started":          map[string]any{"type": "string"},
				"comment":          map[string]any{"type": []string{"object", "null"}, "description": "ADF document; null or absent when the worklog carries no comment."},
			},
		},
	},
})

// WorklogListOutput is `worklog list`'s per-issue envelope data: the issue and
// its worklogs in Jira's native shape. Multi-key reads carry the same object
// at results[].data.
type WorklogListOutput struct {
	Issue    IssueRef        `json:"issue"`
	Worklogs []*jira.Worklog `json:"worklogs"`
}

var _ = register("worklog.list", WorklogListOutput{}, nil)

// EpicListOutput is `epic list`'s envelope data: the standard epic JQL echoed
// back with the epics it returned. The no-profile degraded path returns the
// JQL with an empty epics list so agents can still see the intended query.
type EpicListOutput struct {
	JQL    string        `json:"jql"`
	Epics  []*jira.Issue `json:"epics"`
	Detail bool          `json:"detail"`
}

var _ = register("epic.list", EpicListOutput{}, nil)

// EpicBoardRow is one epic's line in the `epic board` report: its identity and
// its child-issue counts by status.
type EpicBoardRow struct {
	Key     string         `json:"key"`
	Summary string         `json:"summary"`
	Status  string         `json:"status"`
	Counts  map[string]int `json:"counts"`
}

// EpicBoardOutput is `epic board`'s envelope data: one row per epic plus the
// summed status totals across every epic.
type EpicBoardOutput struct {
	Epics  []EpicBoardRow `json:"epics"`
	Totals map[string]int `json:"totals"`
}

var _ = register("epic.board", EpicBoardOutput{}, nil)

// EpicAddOutput is `epic add`'s envelope data: the issue attached, the epic it
// was attached to, and the dry-run/added pair carried on both paths.
type EpicAddOutput struct {
	Issue  IssueRef `json:"issue"`
	Epic   string   `json:"epic"`
	DryRun bool     `json:"dry_run"`
	Added  bool     `json:"added"`
}

var _ = register("epic.add", EpicAddOutput{}, nil)

// EpicRemoveOutput is `epic remove`'s envelope data: the issue detached from
// its epic, with the dry-run/removed pair carried on both paths.
type EpicRemoveOutput struct {
	Issue   IssueRef `json:"issue"`
	DryRun  bool     `json:"dry_run"`
	Removed bool     `json:"removed"`
}

var _ = register("epic.remove", EpicRemoveOutput{}, nil)

// BoardRow is one board's line in `boards list`: the flattened wire shape with
// pointer-typed fields collapsed to their zero values so agents need no
// null-handling for the common board case.
type BoardRow struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	ProjectKeys []string `json:"project_keys"`
}

// BoardsListOutput is `boards list`'s envelope data: the boards plus the
// cache-state trio (cache_state / cache_source_state / cache_empty) and the
// from_cache/fetched_at/truncated markers every cache-backed read reports.
type BoardsListOutput struct {
	Boards           []BoardRow `json:"boards"`
	FromCache        bool       `json:"from_cache"`
	FetchedAt        string     `json:"fetched_at"`
	Truncated        bool       `json:"truncated"`
	TruncatedReason  string     `json:"truncated_reason"`
	CacheState       string     `json:"cache_state"`
	CacheSourceState string     `json:"cache_source_state"`
	CacheEmpty       bool       `json:"cache_empty"`
}

var _ = register("boards.list", BoardsListOutput{}, nil)

// SearchJQLOutput is `search jql`'s envelope data. Issues is polymorphic by
// path: the compact per-issue projection by default, Jira's raw records under
// --full, and absent on the --web preview (which carries url/opened instead of
// running the query). Issues holds any so the field selector can never change
// the field's static type.
type SearchJQLOutput struct {
	Source string `json:"source"`
	JQL    string `json:"jql"`
	Issues any    `json:"issues,omitempty"`
	URL    string `json:"url,omitempty"`
	Opened bool   `json:"opened,omitempty"`
}

var _ = register("search.jql", SearchJQLOutput{}, nil)

// SearchCountOutput is `search jql --count`'s envelope data: Jira's
// approximate match count for the query, with no issues fetched.
type SearchCountOutput struct {
	Source string `json:"source"`
	JQL    string `json:"jql"`
	Count  int    `json:"count"`
}

var _ = register("search.count", SearchCountOutput{}, nil)

// SearchSavedOutput is `search saved`'s envelope data: the named query's
// metadata (from the queries file) and the issues it returned. Issues holds
// the same compact-or-raw projection as search jql, hence any.
type SearchSavedOutput struct {
	Source      string `json:"source"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Project     string `json:"project"`
	JQL         string `json:"jql"`
	Issues      any    `json:"issues"`
}

var _ = register("search.saved", SearchSavedOutput{}, nil)

// JQLBuildOutput is `jql build`'s envelope data: the composed query, a
// best-effort deep link, the board-scope precedence, and the board-scope
// block. BoardScope is a variable-key map (applied plus, when resolved, id /
// name / type / project_keys), so it stays a map.
type JQLBuildOutput struct {
	JQL        string         `json:"jql"`
	URL        string         `json:"url"`
	Precedence string         `json:"precedence"`
	BoardScope map[string]any `json:"board_scope"`
}

var _ = register("jql.build", JQLBuildOutput{}, nil)

// JQLValidateEntry is one query's result from `jql validate`: the query, its
// validity, and Jira's errors/warnings when present. A parse failure is a
// result (valid: false), not a command error.
type JQLValidateEntry struct {
	Query    string   `json:"query"`
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// JQLValidateOutput is `jql validate`'s envelope data: one entry per query.
type JQLValidateOutput struct {
	Queries []JQLValidateEntry `json:"queries"`
}

var _ = register("jql.validate", JQLValidateOutput{}, nil)

// JQLReferenceField is one queryable field in `jql reference`: its JQL value,
// display name, and custom-field id when it is a custom field.
type JQLReferenceField struct {
	Value         string `json:"value"`
	DisplayName   string `json:"display_name"`
	CustomFieldID string `json:"custom_field_id,omitempty"`
}

// JQLReferenceFunction is one JQL function in `jql reference`.
type JQLReferenceFunction struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
}

// JQLReferenceOutput is `jql reference`'s envelope data: the site's queryable
// fields, functions, and reserved words from /jql/autocompletedata.
type JQLReferenceOutput struct {
	Fields        []JQLReferenceField    `json:"fields"`
	Functions     []JQLReferenceFunction `json:"functions"`
	ReservedWords []string               `json:"reserved_words"`
}

var _ = register("jql.reference", JQLReferenceOutput{}, nil)

// MeOutput is `me`'s envelope data: the active profile name and the resolved
// Jira account identity from /myself.
type MeOutput struct {
	Profile      string `json:"profile"`
	AccountID    string `json:"account_id"`
	DisplayName  string `json:"display_name"`
	EmailAddress string `json:"email_address"`
	TimeZone     string `json:"time_zone"`
}

var _ = register("me", MeOutput{}, nil)

// UserSearchMatch is one account in `user search` results: the account_id an
// ADF mention needs, plus the fields a caller disambiguates on.
type UserSearchMatch struct {
	AccountID    string `json:"account_id"`
	DisplayName  string `json:"display_name"`
	EmailAddress string `json:"email_address"`
}

// UserSearchOutput is `user search`'s envelope data: the query echoed back
// with the active, non-deleted account matches and their count.
type UserSearchOutput struct {
	Query string            `json:"query"`
	Users []UserSearchMatch `json:"users"`
	Count int               `json:"count"`
}

var _ = register("user.search", UserSearchOutput{}, nil)
