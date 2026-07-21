package envelope

import (
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Typed Output structs for the issue-core operation family (view, list,
// create, edit, transition, and the destructive clone/move/delete). Each
// struct is the single declaration of its envelope's `data` shape: the
// builder emits it, SchemaOf derives the published schema from it, and the
// registration beside it feeds the exhaustiveness guardrail. Shared identity
// fields ride envelope.IssueRef; a full Jira issue rides *jira.Issue; dynamic
// payload echoes the caller cannot type (validated field maps, the create
// preview, native update blocks) stay map[string]any.

// IssueViewOutput is single-key `issue view`'s envelope data. The multi-key
// form carries an ordered results set instead (a keyed exception the schema
// documents as prose), so only the single-key shape is a fixed struct.
type IssueViewOutput struct {
	Issue *jira.Issue `json:"issue"`
}

var _ = register("issue.view", IssueViewOutput{}, nil)

// IssueListOutput is the union `issue list` emits across its variants: the
// paged/keyed listing (issue.list), the estimate (issue.list.count), and the
// JQL preview (issue.list.jql). One cobra path feeds all three, so a single
// struct covers the union with the variant-only fields omitempty; jql is the
// one field every variant carries. board_scope and the key-chunk echoes stay
// opaque — they are caller/Jira data, not contract surface the type expresses.
type IssueListOutput struct {
	// Issues is the default projected row list; --detail swaps in full
	// issues, and --count omits it. The schema describes the default
	// projection (IssueListRow). The builder emits a map so the bespoke
	// human/compact renderers keep their map contract until the renderer
	// integration lands; this struct is the schema template.
	Issues []IssueListRow `json:"issues,omitempty"`
	// Detail echoes --detail; absent on --count. A pointer so an explicit
	// false is still emitted where the field applies.
	Detail *bool `json:"detail,omitempty"`
	// Jql is the resolved query; present on every variant.
	Jql string `json:"jql"`
	// Precedence and BoardScope report board scoping; absent on --count.
	Precedence string         `json:"precedence,omitempty"`
	BoardScope map[string]any `json:"board_scope,omitempty"`
	// Count is present only on the --count variant (issue.list.count).
	Count *int `json:"count,omitempty"`
	// URL is present only on the --as-jql variant (issue.list.jql).
	URL string `json:"url,omitempty"`
	// SucceededKeyChunks / FailedKeyChunks appear only when a chunked --key
	// listing partially fails.
	SucceededKeyChunks *int `json:"succeeded_key_chunks,omitempty"`
	FailedKeyChunks    any  `json:"failed_key_chunks,omitempty"`
}

// IssueListRow is the projected issue row `issue list` emits by default
// (cmdutil.IssueOutput). assignee and priority are nullable per spec but not
// required; the pointer-omitempty derivation makes them optional, and
// issueListDoc refines them to also accept null.
type IssueListRow struct {
	Key            string             `json:"key"`
	Summary        string             `json:"summary"`
	Status         string             `json:"status"`
	StatusCategory string             `json:"status_category"`
	StatusColor    string             `json:"status_color,omitempty"`
	Assignee       *IssueListAssignee `json:"assignee,omitempty"`
	Priority       *string            `json:"priority,omitempty"`
	Updated        string             `json:"updated"`
}

// IssueListAssignee is the trimmed user identity a projected issue row carries.
type IssueListAssignee struct {
	AccountID   string `json:"account_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// issueListDoc refines the two projected-row fields the pointer derivation
// cannot mark nullable-and-optional at once: assignee and priority are null
// when the issue has none, yet neither is required.
var issueListDoc = map[string]any{
	"properties": map[string]any{
		"issues": map[string]any{
			"items": map[string]any{
				"properties": map[string]any{
					"assignee": map[string]any{"type": []string{"object", "null"}},
					"priority": map[string]any{"type": []string{"string", "null"}},
				},
			},
		},
	},
}

var (
	_ = register("issue.list", IssueListOutput{}, issueListDoc)
	_ = register("issue.list.count", IssueListOutput{}, issueListDoc)
	_ = register("issue.list.jql", IssueListOutput{}, issueListDoc)
)

// IssueCreateOutput is `issue create`'s envelope data across the dry-run
// preview and the live write. dry_run rides both paths; issue is the created
// issue on a live write, preview the would-be payload on a dry-run.
type IssueCreateOutput struct {
	Issue             *jira.Issue    `json:"issue,omitempty"`
	Preview           map[string]any `json:"preview,omitempty"`
	DryRun            bool           `json:"dry_run"`
	ValidatedRemotely bool           `json:"validated_remotely,omitempty"`
	// Verification is the optional --verify re-fetch diff (issue.verificationResult).
	Verification any `json:"verification,omitempty"`
}

var _ = register("issue.create", IssueCreateOutput{}, nil)

// IssueEditOutput is `issue edit`'s envelope data. issue, dry_run, and the
// validated fields echo ride every path; result is the updated issue on a
// live write. update carries a native REST operation block verbatim; warnings
// travels inside each key's data on a fan-out edit.
type IssueEditOutput struct {
	Issue             IssueRef       `json:"issue"`
	Result            *jira.Issue    `json:"result,omitempty"`
	DryRun            bool           `json:"dry_run"`
	Fields            map[string]any `json:"fields"`
	Update            map[string]any `json:"update,omitempty"`
	ValidatedRemotely bool           `json:"validated_remotely,omitempty"`
	Verification      any            `json:"verification,omitempty"`
	Warnings          []adf.Warning  `json:"warnings,omitempty"`
}

var _ = register("issue.edit", IssueEditOutput{}, nil)

// IssueTransitionOutput is `issue transition`'s envelope data when a target is
// applied (or previewed). transition is the resolved id (or the requested
// target on a bare dry-run); the payload sections and transition_validated
// appear only on the dry-run preview paths.
type IssueTransitionOutput struct {
	Issue               IssueRef       `json:"issue"`
	Transition          string         `json:"transition,omitempty"`
	DryRun              bool           `json:"dry_run"`
	Fields              map[string]any `json:"fields,omitempty"`
	Comment             any            `json:"comment,omitempty"`
	Update              map[string]any `json:"update,omitempty"`
	TransitionValidated bool           `json:"transition_validated,omitempty"`
	Warnings            []adf.Warning  `json:"warnings,omitempty"`
}

var _ = register("issue.transition", IssueTransitionOutput{}, nil)

// IssueTransitionsOutput is the no-target `issue transition` read: the issue
// and its available workflow transitions.
type IssueTransitionsOutput struct {
	Issue       IssueRef           `json:"issue"`
	Transitions []*jira.Transition `json:"transitions"`
}

var _ = register("issue.transitions", IssueTransitionsOutput{}, nil)

// IssueDestructiveOutput is the shared envelope data for the clone, move, and
// delete mutations. payload previews the validated fields on a dry-run; result
// is the resulting issue on a live clone/move (delete carries none). warnings
// travels inside each key's data on a fan-out run.
type IssueDestructiveOutput struct {
	Issue    IssueRef       `json:"issue"`
	Result   *jira.Issue    `json:"result,omitempty"`
	Payload  map[string]any `json:"payload,omitempty"`
	DryRun   bool           `json:"dry_run"`
	Warnings []adf.Warning  `json:"warnings,omitempty"`
}

var (
	_ = register("issue.clone", IssueDestructiveOutput{}, nil)
	_ = register("issue.move", IssueDestructiveOutput{}, nil)
	_ = register("issue.delete", IssueDestructiveOutput{}, nil)
)
