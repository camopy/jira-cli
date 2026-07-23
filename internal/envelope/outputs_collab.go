package envelope

// Typed Output structs for the issue-collaboration operation family:
// comments, links, web links, attachments, and watchers. Each struct is the
// single declaration of its envelope's `data` shape, emitted by the builder
// and read back by SchemaOf; the registration beside it feeds the
// exhaustiveness guardrail. Where a command has several code paths that emit
// different key sets (dry-run vs live, readback vs no-readback), one struct
// carries the union and `omitempty` (with pointers where a false/zero/empty
// value must still be present) reproduces today's keys per path exactly.

import (
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// CommentUser is the projected identity carried by a comment author.
type CommentUser struct {
	AccountID    string `json:"account_id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	EmailAddress string `json:"email_address,omitempty"`
}

// CommentItem is the fixed comment projection returned by comment list and
// live comment edit. Body stays dynamic because Jira returns recursive ADF.
type CommentItem struct {
	Author       *CommentUser     `json:"author"`
	Body         any              `json:"body"`
	Created      string           `json:"created,omitempty"`
	ID           string           `json:"id,omitempty"`
	UpdateAuthor *CommentUser     `json:"update_author"`
	Updated      string           `json:"updated,omitempty"`
	Visibility   *jira.Visibility `json:"visibility"`
}

// IssueCommentListOutput is `issue comment list`'s data. The issue and
// comments ride both single-key and keyed reads. Single-key pagination rides
// in meta; keyed reads fold the per-key pagination block and any lossy-ADF or
// rate-limit warnings into the same object at results[].data.
type IssueCommentListOutput struct {
	Issue      IssueRef         `json:"issue"`
	Comments   []CommentItem    `json:"comments"`
	Pagination *Pagination      `json:"pagination,omitempty"`
	Warnings   []map[string]any `json:"warnings,omitempty"`
}

var _ = register("issue.comment.list", IssueCommentListOutput{}, map[string]any{
	"properties": map[string]any{
		"comments": map[string]any{
			"items": map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type":        []string{"object", "null"},
						"description": "The native recursive ADF document, or null when Jira returned no body.",
					},
				},
			},
		},
	},
})

// IssueCommentAddOutput is `issue comment add`'s data (and the group alias
// `issue comment`). `comment` is the created comment on the live path and a
// {body: <ADF>} preview on dry-run, so it stays an opaque object. `warnings`
// appears only on multi-key writes, where command-wide markdown/ADF warnings
// travel with each per-key result.
type IssueCommentAddOutput struct {
	Issue    IssueRef       `json:"issue"`
	Comment  map[string]any `json:"comment"`
	DryRun   bool           `json:"dry_run"`
	Warnings []adf.Warning  `json:"warnings,omitempty"`
}

var (
	_ = register("issue.comment.add", IssueCommentAddOutput{}, nil)
	// The bare `issue comment KEY ...` group form is an alias of comment add
	// and emits the identical shape.
	_ = register("issue.comment", IssueCommentAddOutput{}, nil)
)

// IssueCommentEditOutput is `issue comment edit`'s data. The dry-run preview
// carries the validated body summary and the visibility change; the live
// write echoes the updated comment instead. `comment_id` is present only on
// the preview (the live path identifies the comment via the echoed body).
type IssueCommentEditOutput struct {
	Issue     IssueRef `json:"issue"`
	CommentID string   `json:"comment_id,omitempty"`
	// BodyADFSummary carries the validated *adf.Document on the dry-run path.
	// It is typed `any` (not *adf.Document) because ADF nodes nest
	// recursively and the schema deriver walks types without a cycle guard;
	// the object type is restored through the registration override below.
	BodyADFSummary   any            `json:"body_adf_summary,omitempty"`
	VisibilityChange string         `json:"visibility_change,omitempty"`
	Comment          map[string]any `json:"comment,omitempty"`
	DryRun           bool           `json:"dry_run"`
}

var _ = register("issue.comment.edit", IssueCommentEditOutput{}, map[string]any{
	"properties": map[string]any{
		"body_adf_summary": map[string]any{"type": "object", "description": "Dry-run only: the validated ADF document that would be submitted."},
	},
})

// IssueCommentDeleteOutput is `issue comment delete`'s data. `deleted` rides
// only the live path (always true there); the dry-run preview omits it.
type IssueCommentDeleteOutput struct {
	Issue     IssueRef `json:"issue"`
	CommentID string   `json:"comment_id"`
	Deleted   bool     `json:"deleted,omitempty"`
	DryRun    bool     `json:"dry_run"`
}

var _ = register("issue.comment.delete", IssueCommentDeleteOutput{}, nil)

// IssueLinkCreateOutput is `issue link`'s create data. `type` is always
// present (empty when the native body addressed the type by id only);
// `type_id` and `comment` appear only when the native body supplied them.
type IssueLinkCreateOutput struct {
	InwardIssue  IssueRef       `json:"inward_issue"`
	OutwardIssue IssueRef       `json:"outward_issue"`
	Type         string         `json:"type"`
	TypeID       string         `json:"type_id,omitempty"`
	Comment      map[string]any `json:"comment,omitempty"`
	DryRun       bool           `json:"dry_run"`
}

var _ = register("issue.link", IssueLinkCreateOutput{}, nil)

// IssueLinkDeleteOutput is `issue link delete`'s data. `link_id` echoes the
// supplied id verbatim (links are global); `deleted` rides the live path
// only.
type IssueLinkDeleteOutput struct {
	Issue   IssueRef `json:"issue"`
	LinkID  string   `json:"link_id"`
	Deleted bool     `json:"deleted,omitempty"`
	DryRun  bool     `json:"dry_run"`
}

var _ = register("issue.link.delete", IssueLinkDeleteOutput{}, nil)

// IssueLinkListOutput is `issue link list`'s data: Jira's inward/outward fork
// flattened into one direction-aware array. Multi-key reads carry the same
// object at results[].data.
type IssueLinkListOutput struct {
	Issue IssueRef             `json:"issue"`
	Links []jira.IssueLinkView `json:"links"`
	Count int                  `json:"count"`
}

var _ = register("issue.link.list", IssueLinkListOutput{}, nil)

// IssueLinkTypesOutput is `issue link types`'s data: the configured link
// types plus the cache-provenance trio every cache-backed read carries.
type IssueLinkTypesOutput struct {
	LinkTypes        []jira.IssueLinkType `json:"link_types"`
	Count            int                  `json:"count"`
	FromCache        bool                 `json:"from_cache"`
	FetchedAt        string               `json:"fetched_at"`
	CacheState       string               `json:"cache_state"`
	CacheSourceState string               `json:"cache_source_state"`
	CacheEmpty       bool                 `json:"cache_empty"`
}

var _ = register("issue.link.types", IssueLinkTypesOutput{}, nil)

// IssueWebLinkOutput is `issue weblink`'s data. `url_remote_checked` is
// emitted (always false) only on the dry-run path, to state plainly that the
// preview validated URL syntax locally and never fetched the target — a
// pointer so its presence, not its value, marks the preview.
type IssueWebLinkOutput struct {
	Issue            IssueRef `json:"issue"`
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	DryRun           bool     `json:"dry_run"`
	URLRemoteChecked *bool    `json:"url_remote_checked,omitempty"`
}

var _ = register("issue.weblink", IssueWebLinkOutput{}, nil)

// IssueAttachmentListOutput is `issue attachment list`'s data. Single-key
// reads carry only `attachments` (pagination in meta); multi-key reads fold
// the per-key pagination block in. Each attachment is projected to a fixed
// snake-case shape but kept as an opaque object here.
type IssueAttachmentListOutput struct {
	Attachments []map[string]any `json:"attachments"`
	Pagination  *Pagination      `json:"pagination,omitempty"`
}

var _ = register("issue.attachment.list", IssueAttachmentListOutput{}, nil)

// IssueAttachmentAddOutput is `issue attachment add`'s data. The dry-run
// preview lists the inferred `files`; the live upload lists the created
// `attachments`. Exactly one is present per path.
type IssueAttachmentAddOutput struct {
	Issue       IssueRef         `json:"issue"`
	Files       []map[string]any `json:"files,omitempty"`
	Attachments []map[string]any `json:"attachments,omitempty"`
	DryRun      bool             `json:"dry_run"`
}

var _ = register("issue.attachment.add", IssueAttachmentAddOutput{}, nil)

// IssueAttachmentDeleteOutput is `issue attachment delete`'s data. `deleted`
// rides the live path only.
type IssueAttachmentDeleteOutput struct {
	Issue        IssueRef `json:"issue"`
	AttachmentID string   `json:"attachment_id"`
	Deleted      bool     `json:"deleted,omitempty"`
	DryRun       bool     `json:"dry_run"`
}

var _ = register("issue.attachment.delete", IssueAttachmentDeleteOutput{}, nil)

// IssueAttachmentDownloadOutput is `issue attachment download`'s data. The
// dry-run preview reports the planned write `target` (present even when empty
// in current-dir mode, so a pointer); the live path reports `written_to` and
// the `bytes` written (a pointer so a zero-byte file still emits the count).
// `mode` rides both paths.
type IssueAttachmentDownloadOutput struct {
	Issue        IssueRef `json:"issue"`
	AttachmentID string   `json:"attachment_id"`
	Mode         string   `json:"mode"`
	Target       *string  `json:"target,omitempty"`
	WrittenTo    string   `json:"written_to,omitempty"`
	Bytes        *int64   `json:"bytes,omitempty"`
	DryRun       bool     `json:"dry_run"`
}

var _ = register("issue.attachment.download", IssueAttachmentDownloadOutput{}, nil)

// IssueWatchersListOutput is `issue watchers list`'s data. Multi-key reads
// carry the same object at results[].data.
type IssueWatchersListOutput struct {
	Watchers   []map[string]any `json:"watchers"`
	IsWatching bool             `json:"is_watching"`
	WatchCount int              `json:"watch_count"`
}

var _ = register("issue.watchers.list", IssueWatchersListOutput{}, nil)

// IssueWatcherMutationOutput is the data for `issue watchers add` and
// `issue watchers remove` (and the watch/unwatch shortcuts) — one shape, so
// consumers never branch on which verb ran. Three code paths emit disjoint
// key sets, and the pointer fields keep a false/zero/empty value present on
// the path that carries it while staying absent on the others:
//
//   - dry-run:     issue, user, dry_run, user_resolved, account_id_resolved?
//   - no-readback: issue, account_id, attempted, dry_run
//   - readback:    issue, watchers, is_watching, watch_count,
//     was_already_watching, dry_run
type IssueWatcherMutationOutput struct {
	Issue              IssueRef          `json:"issue"`
	User               string            `json:"user,omitempty"`
	UserResolved       *bool             `json:"user_resolved,omitempty"`
	AccountIDResolved  string            `json:"account_id_resolved,omitempty"`
	AccountID          string            `json:"account_id,omitempty"`
	Attempted          bool              `json:"attempted,omitempty"`
	Watchers           *[]map[string]any `json:"watchers,omitempty"`
	IsWatching         *bool             `json:"is_watching,omitempty"`
	WatchCount         *int              `json:"watch_count,omitempty"`
	WasAlreadyWatching *bool             `json:"was_already_watching,omitempty"`
	DryRun             bool              `json:"dry_run"`
}

var (
	_ = register("issue.watchers.add", IssueWatcherMutationOutput{}, nil)
	_ = register("issue.watchers.remove", IssueWatcherMutationOutput{}, nil)
)
