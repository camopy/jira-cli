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

// IssueCommentEditOutput is `issue comment edit`'s data. The validated body
// summary, comment identity, and visibility change ride both paths; the live
// write also carries the updated comment returned by Jira.
type IssueCommentEditOutput struct {
	Issue     IssueRef `json:"issue"`
	CommentID string   `json:"comment_id"`
	// BodyADFSummary carries the validated *adf.Document on both paths.
	// It is typed `any` (not *adf.Document) because ADF nodes nest
	// recursively and the schema deriver walks types without a cycle guard;
	// the object type is restored through the registration override below.
	BodyADFSummary   any          `json:"body_adf_summary"`
	VisibilityChange string       `json:"visibility_change"`
	Comment          *CommentItem `json:"comment,omitempty"`
	DryRun           bool         `json:"dry_run"`
}

var _ = register("issue.comment.edit", IssueCommentEditOutput{}, map[string]any{
	"properties": map[string]any{
		"body_adf_summary": map[string]any{"type": "object", "description": "The validated ADF document proposed or submitted by the command."},
		"comment":          map[string]any{"description": "The updated comment returned by Jira after a successful live write."},
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
// `preview` renders the sentence each endpoint's page will display and
// appears when the link type's phrase pair is known — always on a live
// create (a cache miss triggers one /issueLinkType fetch), and from the
// per-profile linktypes cache on `--dry-run`, which stays offline.
type IssueLinkCreateOutput struct {
	InwardIssue  IssueRef          `json:"inward_issue"`
	OutwardIssue IssueRef          `json:"outward_issue"`
	Type         string            `json:"type"`
	TypeID       string            `json:"type_id,omitempty"`
	Comment      map[string]any    `json:"comment,omitempty"`
	Preview      *IssueLinkPreview `json:"preview,omitempty"`
	DryRun       bool              `json:"dry_run"`
}

// IssueLinkPreview names the sentence each issue's own page will render once
// the link exists. The field names anchor to the ISSUE, not the phrase: the
// inward issue displays the type's outward phrase and vice versa, which is
// exactly the crossover the preview exists to make visible before creating.
type IssueLinkPreview struct {
	InwardIssueSentence  string `json:"inward_issue_sentence"`
	OutwardIssueSentence string `json:"outward_issue_sentence"`
}

var _ = register("issue.link", IssueLinkCreateOutput{}, map[string]any{
	"properties": map[string]any{
		"preview": map[string]any{"description": "Present when the link type's inward/outward phrases are known: always on a live create, and when the linktypes cache is primed (jira cache linktypes) on --dry-run. inward_issue_sentence is the line KEY's own page will show; outward_issue_sentence is the line the --to issue's page will show."},
	},
})

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

// IssueWebLinkOutput is `issue weblink`'s data. url_remote_checked is always
// false because both paths validate URL syntax locally without fetching the
// target.
type IssueWebLinkOutput struct {
	Issue            IssueRef `json:"issue"`
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	URLRemoteChecked bool     `json:"url_remote_checked"`
	DryRun           bool     `json:"dry_run"`
}

var _ = register("issue.weblink", IssueWebLinkOutput{}, map[string]any{
	"properties": map[string]any{
		"url_remote_checked": map[string]any{"description": "Whether the command fetched the target URL; always false."},
	},
})

// AttachmentAuthor is the fixed uploader identity projected on attachments.
type AttachmentAuthor struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
}

// AttachmentItem is the fixed attachment projection returned by list and add.
type AttachmentItem struct {
	Author   AttachmentAuthor `json:"author"`
	Created  string           `json:"created"`
	Filename string           `json:"filename"`
	ID       string           `json:"id"`
	MIMEType string           `json:"mime_type"`
	Size     int64            `json:"size"`
}

// IssueAttachmentListOutput is `issue attachment list`'s data. The issue and
// attachments ride both single-key and keyed reads. Single-key pagination
// rides in meta; keyed reads fold the per-key pagination block into data.
type IssueAttachmentListOutput struct {
	Issue       IssueRef         `json:"issue"`
	Attachments []AttachmentItem `json:"attachments"`
	Pagination  *Pagination      `json:"pagination,omitempty"`
}

var _ = register("issue.attachment.list", IssueAttachmentListOutput{}, nil)

// AttachmentFile is the locally validated file metadata retained by both
// attachment-add paths.
type AttachmentFile struct {
	MIMEInferred string `json:"mime_inferred"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
}

// IssueAttachmentAddOutput is `issue attachment add`'s data. files carries
// the validated upload context on both paths; attachments is the live outcome.
type IssueAttachmentAddOutput struct {
	Issue       IssueRef         `json:"issue"`
	Files       []AttachmentFile `json:"files"`
	Attachments []AttachmentItem `json:"attachments,omitempty"`
	DryRun      bool             `json:"dry_run"`
}

var _ = register("issue.attachment.add", IssueAttachmentAddOutput{}, map[string]any{
	"properties": map[string]any{
		"files":       map[string]any{"description": "Locally validated files proposed or submitted by the command."},
		"attachments": map[string]any{"description": "Attachments returned by Jira after a successful live upload."},
	},
})

// IssueAttachmentDeleteOutput is `issue attachment delete`'s data. `deleted`
// rides the live path only.
type IssueAttachmentDeleteOutput struct {
	Issue        IssueRef `json:"issue"`
	AttachmentID string   `json:"attachment_id"`
	Deleted      bool     `json:"deleted,omitempty"`
	DryRun       bool     `json:"dry_run"`
}

var _ = register("issue.attachment.delete", IssueAttachmentDeleteOutput{}, nil)

// IssueAttachmentDownloadOutput is `issue attachment download`'s data. target
// retains the originally requested destination on both paths; written_to and
// bytes report the live outcome. A zero-byte file still emits bytes via the
// pointer.
type IssueAttachmentDownloadOutput struct {
	Issue        IssueRef `json:"issue"`
	AttachmentID string   `json:"attachment_id"`
	Mode         string   `json:"mode"`
	Target       string   `json:"target"`
	WrittenTo    string   `json:"written_to,omitempty"`
	Bytes        *int64   `json:"bytes,omitempty"`
	DryRun       bool     `json:"dry_run"`
}

var _ = register("issue.attachment.download", IssueAttachmentDownloadOutput{}, map[string]any{
	"properties": map[string]any{
		"target":     map[string]any{"description": "The destination requested before any server-derived filename is applied."},
		"written_to": map[string]any{"description": "The path written by a successful live download."},
		"bytes":      map[string]any{"description": "The number of bytes written by a successful live download."},
	},
})

// WatcherItem is the fixed user projection returned by watcher reads.
type WatcherItem struct {
	AccountID    string  `json:"account_id"`
	DisplayName  string  `json:"display_name"`
	EmailAddress *string `json:"email_address,omitempty"`
}

// IssueWatchersListOutput is `issue watchers list`'s data. Single-key and
// keyed reads carry the same issue-scoped object.
type IssueWatchersListOutput struct {
	Issue      IssueRef      `json:"issue"`
	Watchers   []WatcherItem `json:"watchers"`
	IsWatching bool          `json:"is_watching"`
	WatchCount int           `json:"watch_count"`
}

var _ = register("issue.watchers.list", IssueWatchersListOutput{}, nil)

// IssueWatcherMutationOutput is the data for `issue watchers add` and
// `issue watchers remove` (and the watch/unwatch shortcuts) — one shape, so
// consumers never branch on which verb ran. User and resolution context ride
// every path; the remaining pointer fields keep a false/zero/empty outcome
// present on the path that carries it while staying absent on the others:
//
//   - dry-run:     issue, user, dry_run, user_resolved, account_id_resolved?
//   - no-readback: stable context plus account_id, attempted
//   - readback:    stable context plus watchers, is_watching, watch_count,
//     was_already_watching, dry_run
type IssueWatcherMutationOutput struct {
	Issue              IssueRef       `json:"issue"`
	User               string         `json:"user"`
	UserResolved       bool           `json:"user_resolved"`
	AccountIDResolved  string         `json:"account_id_resolved,omitempty"`
	AccountID          string         `json:"account_id,omitempty"`
	Attempted          bool           `json:"attempted,omitempty"`
	Watchers           *[]WatcherItem `json:"watchers,omitempty"`
	IsWatching         *bool          `json:"is_watching,omitempty"`
	WatchCount         *int           `json:"watch_count,omitempty"`
	WasAlreadyWatching *bool          `json:"was_already_watching,omitempty"`
	DryRun             bool           `json:"dry_run"`
}

var (
	watcherMutationDoc = map[string]any{
		"properties": map[string]any{
			"user":                map[string]any{"description": "The original user identifier supplied by the caller."},
			"user_resolved":       map[string]any{"description": "Whether the user identifier was resolved to a Jira account ID."},
			"account_id_resolved": map[string]any{"description": "The resolved Jira account ID, when resolution succeeded."},
			"account_id":          map[string]any{"description": "Legacy no-readback outcome containing the attempted account ID."},
			"attempted":           map[string]any{"description": "No-readback outcome confirming the mutation request was attempted."},
			"watchers":            map[string]any{"description": "Watcher list returned by the optional post-mutation readback."},
			"is_watching":         map[string]any{"description": "Watching state returned by the optional post-mutation readback."},
			"watch_count":         map[string]any{"description": "Watcher count returned by the optional post-mutation readback."},
			"was_already_watching": map[string]any{
				"description": "Whether the requested state already held, when post-mutation readback can determine it.",
			},
		},
	}
	_ = register("issue.watchers.add", IssueWatcherMutationOutput{}, watcherMutationDoc)
	_ = register("issue.watchers.remove", IssueWatcherMutationOutput{}, watcherMutationDoc)
)
