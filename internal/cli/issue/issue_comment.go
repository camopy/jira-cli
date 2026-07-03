package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

// issueCommentGroup is the new cobra surface for the comment lifecycle.
// Subcommands list / add / edit / delete cover the comment surface.
//
// Back-compat: `jira issue comment KEY ...` (today's invocation) MUST keep
// working as an alias of `jira issue comment add KEY ...`. cobra's natural
// fallback handles this — when args[0] doesn't match a subcommand name,
// cobra invokes the parent's RunE with all positional args. The group's
// RunE delegates to runCommentAdd so the alias and the explicit
// `comment add` path build identical wire requests.
//
// `dynamic-args='issuekey'` is wired on every command that takes KEY
// positionally so the future issue-key cache plugs in here without further
// command-level changes.
func issueCommentGroup() *cobra.Command {
	addFlags := commentAddFlags{}
	cmd := &cobra.Command{
		Use:   "comment KEY...",
		Short: "Manage comments on a Jira issue",
		Long: "Add, list, edit, or delete Jira issue comments. Use the subcommands for " +
			"explicit comment workflows, or pass issue keys directly for the legacy " +
			"`comment add` behavior.\n\n" +
			"Comment bodies can be supplied as Markdown convenience input or native ADF " +
			"JSON. Mutating comment commands run the body through the validate-and-encode " +
			"ADF pipeline before submission.",
		Example: `# Add a comment using the legacy shorthand
$ jira issue comment PROJ-123 --body-markdown "Deployed to staging."

# Add a comment from a native ADF JSON file
$ jira issue comment PROJ-123 --json-input ./comment.json

# Preview the comment without contacting Jira
$ jira issue comment add PROJ-123 --body-markdown "Draft note" --dry-run`,
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runCommentAddKeys(cmd, keys, addFlags)
		},
	}
	registerCommentAddFlags(cmd, &addFlags)
	cmd.AddCommand(commentListCommand())
	cmd.AddCommand(commentAddCommand())
	cmd.AddCommand(commentEditCommand())
	cmd.AddCommand(commentDeleteCommand())
	return cmd
}

// ---------- comment list ----------

func commentListCommand() *cobra.Command {
	var limit int
	var all bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "list KEY...",
		Short: "List comments on an issue",
		Long: "List comments on one or more issues. Use it to inspect discussion history " +
			"or find comment IDs before editing or deleting.\n\n" +
			"`--all` walks comment pages until Jira reports the last page or a rate limit " +
			"interrupts pagination. ADF bodies are rendered to Markdown for human output; " +
			"lossy renders are surfaced as warnings.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.MinimumNArgs(1),
		Example: `# List the most recent comments on an issue
$ jira issue comment list PROJ-123

# Walk every page of comments
$ jira issue comment list PROJ-123 --all

# Keep comment metadata parseable
$ jira issue comment list PROJ-123 --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			if len(keys) == 1 {
				return runCommentList(cmd, keys[0], limit, all)
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.comment.list")
			}
			svc := cmdutil.ServicesForClient(client).Comment()
			results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (commentListReadResult, error) {
				return commentListEnvelopeData(ctx, svc, key, limit, all)
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.comment.list", results, func(_ string, result commentListReadResult) any {
				data := cmdutil.CopyAnyMap(result.Data)
				if len(result.Warnings) > 0 {
					data["warnings"] = result.Warnings
				}
				return data
			})
		},
	}
	cmdutil.AddIntVar(cmd.Flags(), &limit, "limit", 50, "Maximum comments per page", clib.FlagExtra{Group: "Pagination", Placeholder: "N"})
	cmdutil.AddBoolVar(cmd.Flags(), &all, "all", false, "Walk every page until isLast", clib.FlagExtra{Group: "Pagination"})
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

// runCommentList fetches `/issue/{key}/comment` and emits the standard
// envelope under --json/--compact, raw Atlassian shape under --raw, or a
// plain table otherwise. With --all, the service-layer drain handles
// pagination and reports rate-limit-during-paginate as partial success.
func runCommentList(cmd *cobra.Command, key string, limit int, all bool) error {
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.list")
	}
	var result commentListReadResult
	if err := cmdutil.Spin(cmd, "issue.comment.list", func(ctx context.Context) error {
		var listErr error
		result, listErr = commentListEnvelopeData(ctx, cmdutil.ServicesForClient(client).Comment(), key, limit, all)
		return listErr
	}); err != nil {
		return err
	}
	return writeEnvelopeWithCommentWarnings(cmd, "issue.comment.list", result.Data, result.Warnings)
}

type commentListReadResult struct {
	Data     map[string]any
	Warnings []map[string]any
}

func commentListEnvelopeData(ctx context.Context, svc jira.CommentService, key string, limit int, all bool) (commentListReadResult, error) {
	pageSize := limit
	if pageSize <= 0 {
		pageSize = 50
	}

	var (
		collected    []*jira.Comment
		lastResp     *jira.Response
		pagesFetched int
		rateLimitHit *jira.APIError
	)
	if all {
		drained, err := svc.ListAll(ctx, key, jira.CommentDrainOptions{PageSize: pageSize})
		if err != nil {
			return commentListReadResult{}, err
		}
		collected = drained.Comments
		lastResp = drained.LastResp
		pagesFetched = drained.PagesFetched
		rateLimitHit = drained.RateLimitHit
	} else {
		comments, resp, err := svc.List(ctx, key, &jira.ListCommentsOptions{
			ListOptions: jira.ListOptions{MaxResults: pageSize},
		})
		if err != nil {
			return commentListReadResult{}, err
		}
		collected = comments
		lastResp = resp
		pagesFetched = 1
	}

	pagination := commentListPagination(lastResp, all, rateLimitHit)
	commentsOut := commentListData(collected)
	data := map[string]any{
		"comments":   commentsOut,
		"pagination": pagination,
	}

	warnings := commentListWarnings(collected, rateLimitHit, pagesFetched)
	return commentListReadResult{Data: data, Warnings: warnings}, nil
}

// commentListPagination produces the snake-case pagination block per
// envelope-shapes.md. : when rate-limit hit mid --all, is_last is false
// and next_page_token carries the cursor to resume from.
func commentListPagination(resp *jira.Response, all bool, rateLimitHit *jira.APIError) map[string]any {
	if resp == nil {
		return map[string]any{
			"total":           0,
			"start_at":        0,
			"max_results":     0,
			"is_last":         true,
			"next_page_token": nil,
		}
	}
	isLast := resp.IsLast
	if rateLimitHit != nil {
		isLast = false
	}
	if all && rateLimitHit == nil {
		// After a clean --all walk we always landed on isLast=true.
		isLast = true
	}
	out := map[string]any{
		"total":           resp.Total,
		"start_at":        resp.StartAt, // pagination-exempt: output-shape passthrough.
		"max_results":     resp.MaxResults,
		"is_last":         isLast,
		"next_page_token": nil,
	}
	if !isLast {
		out["next_page_token"] = resp.NextCursor()
	}
	return out
}

// commentListData renders each comment with snake-case keys per
// envelope-shapes.md. Bodies stay native ADF — read/reuse parity with
// issue view, so mention accountIds, cards, and every other node survive;
// the plain renderer flattens for human display.
func commentListData(comments []*jira.Comment) []map[string]any {
	out := make([]map[string]any, 0, len(comments))
	for _, c := range comments {
		out = append(out, commentToMap(c))
	}
	return out
}

func commentToMap(c *jira.Comment) map[string]any {
	if c == nil {
		return nil
	}
	m := map[string]any{}
	if c.ID != nil {
		m["id"] = *c.ID
	}
	if c.Body != nil {
		m["body"] = *c.Body
	} else {
		m["body"] = nil
	}
	m["author"] = userToMap(c.Author)
	m["update_author"] = userToMap(c.UpdateAuthor)
	if c.Created != nil {
		m["created"] = *c.Created
	}
	if c.Updated != nil {
		m["updated"] = *c.Updated
	}
	if c.Visibility != nil {
		m["visibility"] = map[string]any{
			"type":  c.Visibility.Type,
			"value": c.Visibility.Value,
		}
	} else {
		m["visibility"] = nil
	}
	return m
}

func userToMap(u *jira.User) map[string]any {
	if u == nil {
		return nil
	}
	m := map[string]any{}
	if u.AccountID != nil {
		m["account_id"] = *u.AccountID
	}
	if u.DisplayName != nil {
		m["display_name"] = *u.DisplayName
	}
	if u.EmailAddress != nil {
		m["email_address"] = *u.EmailAddress
	}
	return m
}

// commentListWarnings combines two warning sources:
//   - per-comment lossy ADF
//   - rate-limit-during-paginate
func commentListWarnings(comments []*jira.Comment, rateLimitHit *jira.APIError, pagesFetched int) []map[string]any {
	var out []map[string]any
	for _, lw := range jira.CollectLossyCommentWarnings(comments) {
		out = append(out, map[string]any{
			"type":             lw.Type,
			"comment_id":       lw.CommentID,
			"lossy_constructs": lw.LossyConstructs,
		})
	}
	if rateLimitHit != nil {
		warning := map[string]any{
			"type":                "rate-limit-during-paginate",
			"pages_fetched":       pagesFetched,
			"retry_after_seconds": rateLimitHit.RetryAfterSeconds,
		}
		if rateLimitHit.StatusCode != 0 {
			warning["http_status"] = rateLimitHit.StatusCode
		}
		out = append(out, warning)
	}
	return out
}

// writeEnvelopeWithCommentWarnings emits the envelope with snake-case
// per-comment warnings under warnings[]. We don't reuse
// cmdutil.WriteEnvelopeWithWarnings because that helper is shaped around adf.Warning
// (Type/Message/Field/NodeType etc.) — comment-list warnings are a different
// schema.
func writeEnvelopeWithCommentWarnings(cmd *cobra.Command, command string, data any, warnings []map[string]any) error {
	if cmdutil.UseCompactOutput(cmd) {
		return cli.WriteCompact(cmd.OutOrStdout(), cmdutil.FoldRawWarningsIntoData(data, warnings))
	}
	if cmdutil.UsePlainOutput(cmd) {
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, cmdutil.PlainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return cmdutil.MirrorADFWarningsToStderr(cmd.ErrOrStderr(), cmdutil.RawWarningsToCLI(warnings))
	}
	body := map[string]any{
		"ok": true,
		"meta": map[string]any{
			"command":    command,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": cli.NewRequestID(),
		},
		"data":     data,
		"errors":   []any{},
		"warnings": warningsOrEmpty(warnings),
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	return enc.Encode(body)
}

func warningsOrEmpty(w []map[string]any) []map[string]any {
	if w == nil {
		return []map[string]any{}
	}
	return w
}

// ---------- comment add ----------

type commentAddFlags struct {
	markdown    string
	jsonInput   string
	visRole     string
	visGroup    string
	parallelism int
	dryRun      bool
}

func commentAddCommand() *cobra.Command {
	flags := commentAddFlags{}
	cmd := &cobra.Command{
		Use:   "add KEY...",
		Short: "Add a comment to an issue",
		Long: "Add the same comment to one or more issues. Use Markdown for quick terminal " +
			"notes or `--json-input` when an agent already has a native ADF comment body.\n\n" +
			"The comment body runs through the validate-and-encode ADF pipeline before " +
			"submission. `--dry-run` previews the validated ADF and does not contact Jira. " +
			"Visibility can be restricted by Jira role or group.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.MinimumNArgs(1),
		Example: `# Add a Markdown comment to an issue
$ jira issue comment add PROJ-123 --body-markdown "Deployed to staging."

# Add a comment from a native ADF JSON file
$ jira issue comment add PROJ-123 --json-input ./comment.json

# Restrict a comment to a Jira role
$ jira issue comment add PROJ-123 --body-markdown "Internal note." --visibility-role Developers

# Preview a comment for an agent
$ jira issue comment add PROJ-123 --body-markdown "Draft note." --dry-run --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runCommentAddKeys(cmd, keys, flags)
		},
	}
	registerCommentAddFlags(cmd, &flags)
	return cmd
}

func registerCommentAddFlags(cmd *cobra.Command, flags *commentAddFlags) {
	cmdutil.AddStringVar(cmd.Flags(), &flags.markdown, "body-markdown", "", "Comment body as Markdown (lossy convenience layer)", clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	cmdutil.AddFileFlag(cmd.Flags(), &flags.jsonInput, "json-input", "", "Comment body as native ADF JSON file (canonical for agents)", "Input", "FILE")
	cmdutil.AddStringVar(cmd.Flags(), &flags.visRole, "visibility-role", "", "Restrict comment to a Jira role (e.g. Developers)", clib.FlagExtra{Group: "Visibility", Placeholder: "ROLE"})
	cmdutil.AddStringVar(cmd.Flags(), &flags.visGroup, "visibility-group", "", "Restrict comment to a Jira group", clib.FlagExtra{Group: "Visibility", Placeholder: "GROUP"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &flags.dryRun, "Preview mutation without submitting")
	cmdutil.AddParallelismFlag(cmd, &flags.parallelism)
	// Exactly one body source: a Markdown convenience string or a native
	// ADF JSON file. Declared as Cobra flag metadata so the conflict is
	// rejected before RunE reads either source.
	cmd.MarkFlagsMutuallyExclusive("body-markdown", "json-input")
}

func runCommentAddKeys(cmd *cobra.Command, keys []string, flags commentAddFlags) error {
	doc, markdownWarnings, err := buildCommentBody(cmd, flags.markdown, flags.jsonInput, cmdutil.NoInputRequested(cmd))
	if err != nil {
		return err
	}
	vis, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		RoleSet:  cmd.Flags().Changed("visibility-role"),
		Role:     flags.visRole,
		GroupSet: cmd.Flags().Changed("visibility-group"),
		Group:    flags.visGroup,
	})
	if err != nil {
		return err
	}
	pipeOut := pipeline.RunMutation(pipeline.MutationInput{
		Mode:             cmdutil.ADFModeFor(cmd, true),
		ADFDoc:           &doc,
		MarkdownWarnings: markdownWarnings,
		DryRun:           flags.dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit and preview the validated SubmitADF, not the
	// pre-pipeline document. ADFDoc was non-nil above, so the pipeline
	// always sets SubmitADF.
	submitDoc := pipeOut.SubmitADF
	if len(keys) > 1 {
		return runCommentAddMany(cmd, keys, flags.parallelism, submitDoc, vis, pipeOut.Warnings, flags.dryRun)
	}
	key := keys[0]
	if flags.dryRun {
		return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.comment.add", map[string]any{
			"issue":   key,
			"comment": map[string]any{"body": submitDoc},
			"dry_run": true,
		}, pipeOut.Warnings)
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.add")
	}
	body := &jira.CommentBody{ADF: submitDoc}
	svc := cmdutil.ServicesForClient(client).Comment()
	var (
		comment *jira.Comment
		resp    *jira.Response
	)
	if err := cmdutil.Spin(cmd, "issue.comment.add", func(ctx context.Context) error {
		var addErr error
		if vis.Mode == jira.VisibilityReplace {
			comment, resp, addErr = svc.AddWithVisibility(ctx, key, body, vis)
		} else {
			comment, resp, addErr = svc.Add(ctx, key, body)
		}
		return addErr
	}); err != nil {
		return err
	}
	return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.comment.add", map[string]any{
		"issue":   key,
		"comment": commentToMap(comment),
		"dry_run": false,
	}, resp, pipeOut.Warnings)
}

func runCommentAddMany(
	cmd *cobra.Command,
	keys []string,
	parallelism int,
	submitDoc *adf.Document,
	vis jira.VisibilityChange,
	warnings []adf.Warning,
	dryRun bool,
) error {
	if dryRun {
		results := make([]cmdutil.KeyResult[map[string]any], len(keys))
		for i, key := range keys {
			results[i] = cmdutil.KeyResult[map[string]any]{
				Key: key,
				Value: map[string]any{
					"issue":   key,
					"comment": map[string]any{"body": submitDoc},
					"dry_run": true,
				},
			}
		}
		return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.comment.add", results, cmdutil.KeyedDataWithWarnings(warnings))
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.add")
	}
	body := &jira.CommentBody{ADF: submitDoc}
	svc := cmdutil.ServicesForClient(client).Comment()
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		var (
			comment *jira.Comment
			addErr  error
		)
		if vis.Mode == jira.VisibilityReplace {
			comment, _, addErr = svc.AddWithVisibility(ctx, key, body, vis)
		} else {
			comment, _, addErr = svc.Add(ctx, key, body)
		}
		if addErr != nil {
			return nil, addErr
		}
		return map[string]any{
			"issue":   key,
			"comment": commentToMap(comment),
			"dry_run": false,
		}, nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.comment.add", results, cmdutil.KeyedDataWithWarnings(warnings))
}

// buildCommentBody parses --body-markdown / --json-input into an ADF doc.
// Pre-flight enforces:
//   - the resulting ADF is non-empty
//   - --no-input requires explicit body input
//
// The body-markdown / json-input exclusivity is enforced declaratively
// by cmd.MarkFlagsMutuallyExclusive, so it never reaches here.
func buildCommentBody(cmd *cobra.Command, markdown, jsonInput string, noInput bool) (adf.Document, []adf.Warning, error) {
	markdownSet := cmd != nil && cmd.Flags().Changed("body-markdown")
	// Empty/missing body: prefer the explicit "body is required" wording so
	// the validation error surfaces consistently. The --no-input rider still
	// appears when the caller passed neither flag *and* opted out of prompts.
	if xstrings.IsBlank(markdown) && jsonInput == "" {
		switch {
		case markdownSet:
			return adf.Document{}, nil, fmt.Errorf("validation: comment body is required: --body-markdown is empty")
		case noInput:
			return adf.Document{}, nil, fmt.Errorf("validation: comment body is required: --no-input requires --body-markdown or --json-input")
		default:
			return adf.Document{}, nil, fmt.Errorf("validation: comment body is required (use --body-markdown or --json-input)")
		}
	}
	if jsonInput != "" {
		var payload map[string]any
		if err := cmdutil.ReadJSONFile(jsonInput, &payload); err != nil {
			return adf.Document{}, nil, err
		}
		if body, ok := payload["body"].(map[string]any); ok {
			payload = body
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return adf.Document{}, nil, err
		}
		parsed, _, err := adf.Parse(raw)
		if err != nil {
			return adf.Document{}, nil, fmt.Errorf("comment --json-input parse: %w", err)
		}
		if len(parsed.Content) == 0 {
			return adf.Document{}, nil, fmt.Errorf("validation: comment body is required: --json-input doc has no content")
		}
		return parsed, nil, nil
	}
	if xstrings.IsBlank(markdown) {
		return adf.Document{}, nil, fmt.Errorf("validation: comment body is required: --body-markdown is empty")
	}
	doc, warnings, err := adf.FromMarkdownLossy(markdown)
	if err != nil {
		return adf.Document{}, nil, err
	}
	if len(doc.Content) == 0 {
		return adf.Document{}, nil, fmt.Errorf("validation: comment body is required: Markdown produced empty ADF")
	}
	return doc, warnings, nil
}

// ---------- comment edit ----------

type commentEditFlags struct {
	markdown  string
	jsonInput string
	visRole   string
	visGroup  string
	visClear  bool
	dryRun    bool
}

func commentEditCommand() *cobra.Command {
	flags := commentEditFlags{}
	cmd := &cobra.Command{
		Use:   "edit KEY COMMENT_ID",
		Short: "Edit an existing comment",
		Long: "Replace a comment body and optionally adjust its visibility. Use `comment " +
			"list` first when you need to find the comment ID.\n\n" +
			"The replacement body runs through the validate-and-encode ADF pipeline before " +
			"submission. `--dry-run` previews the validated body and visibility change " +
			"without contacting Jira.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(2),
		Example: `# Replace a comment body with new Markdown
$ jira issue comment edit PROJ-123 10042 --body-markdown "Updated: rollout complete."

# Remove a comment's visibility restriction
$ jira issue comment edit PROJ-123 10042 --body-markdown "Now public." --clear-visibility

# Preview a comment edit
$ jira issue comment edit PROJ-123 10042 --body-markdown "Draft update." --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommentEdit(cmd, args[0], args[1], flags)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &flags.markdown, "body-markdown", "", "New body as Markdown", clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	cmdutil.AddFileFlag(cmd.Flags(), &flags.jsonInput, "json-input", "", "New body as native ADF JSON file (canonical for agents)", "Input", "FILE")
	cmdutil.AddStringVar(cmd.Flags(), &flags.visRole, "visibility-role", "", "Replace visibility with a Jira role", clib.FlagExtra{Group: "Visibility", Placeholder: "ROLE"})
	cmdutil.AddStringVar(cmd.Flags(), &flags.visGroup, "visibility-group", "", "Replace visibility with a Jira group", clib.FlagExtra{Group: "Visibility", Placeholder: "GROUP"})
	cmdutil.AddBoolVar(cmd.Flags(), &flags.visClear, "clear-visibility", false, "Remove any existing visibility restriction", clib.FlagExtra{Group: "Visibility"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &flags.dryRun, "Preview without calling Jira")
	cmd.MarkFlagsMutuallyExclusive("body-markdown", "json-input")
	return cmd
}

func runCommentEdit(cmd *cobra.Command, key, commentID string, flags commentEditFlags) error {
	vis, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		RoleSet:  cmd.Flags().Changed("visibility-role"),
		Role:     flags.visRole,
		GroupSet: cmd.Flags().Changed("visibility-group"),
		Group:    flags.visGroup,
		Clear:    flags.visClear,
	})
	if err != nil {
		return err
	}
	doc, markdownWarnings, err := buildCommentBody(cmd, flags.markdown, flags.jsonInput, cmdutil.NoInputRequested(cmd))
	if err != nil {
		return err
	}
	pipeOut := pipeline.RunMutation(pipeline.MutationInput{
		Mode:             cmdutil.ADFModeFor(cmd, true),
		ADFDoc:           &doc,
		MarkdownWarnings: markdownWarnings,
		DryRun:           flags.dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit and preview the validated SubmitADF. ADFDoc was
	// non-nil above, so the pipeline always sets SubmitADF.
	submitDoc := pipeOut.SubmitADF
	if flags.dryRun {
		return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.comment.edit", map[string]any{
			"issue":             key,
			"comment_id":        commentID,
			"body_adf_summary":  submitDoc,
			"visibility_change": describeVisibilityChange(vis),
			"dry_run":           true,
		}, pipeOut.Warnings)
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.edit")
	}
	var (
		comment *jira.Comment
		resp    *jira.Response
	)
	if err := cmdutil.Spin(cmd, "issue.comment.edit", func(ctx context.Context) error {
		var editErr error
		comment, resp, editErr = cmdutil.ServicesForClient(client).Comment().Edit(ctx, key, commentID, &jira.CommentBody{ADF: submitDoc}, vis)
		return editErr
	}); err != nil {
		return err
	}
	return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.comment.edit", map[string]any{
		"issue":   key,
		"comment": commentToMap(comment),
		"dry_run": false,
	}, resp, pipeOut.Warnings)
}

func describeVisibilityChange(vis jira.VisibilityChange) string {
	switch vis.Mode {
	case jira.VisibilityReplace:
		return "replace"
	case jira.VisibilityClear:
		return "clear"
	default:
		return "keep"
	}
}

// ---------- comment delete ----------

func commentDeleteCommand() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete KEY COMMENT_ID",
		Short: "Delete a comment",
		Long: "Delete one comment by ID from an issue. Use it after `comment list` when a " +
			"comment should be removed rather than edited.\n\n" +
			"`--dry-run` previews the deletion and never contacts Jira. Live deletes require " +
			"`--force` in headless, agent, or `--no-input` mode; an interactive terminal is " +
			"prompted when `--force` is omitted.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(2),
		Example: `# Preview deleting a comment
$ jira issue comment delete PROJ-123 10042 --dry-run

# Delete a comment without an interactive prompt
$ jira issue comment delete PROJ-123 10042 --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "issue.comment.delete", map[string]any{
					"issue":      args[0],
					"comment_id": args[1],
					"dry_run":    true,
				})
			}
			// Destructive op safety — same shape as `attachment delete`,
			// `link delete`, and `destructiveIssueCommand`. Headless /
			// agent / --no-input MUST pass --force; TTY humans get an
			// interactive confirmation when --force is omitted.
			det := cmdutil.DetectorFromContext(cmd)
			if !force {
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("issue comment delete requires --force in headless / agent / --no-input mode")
				}
				if ok, err := confirmDestructive(cmd, "comment delete", args[1]); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "comment delete", nil)
				}
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.comment.delete")
			}
			var resp *jira.Response
			if err := cmdutil.Spin(cmd, "issue.comment.delete", func(ctx context.Context) error {
				var delErr error
				resp, delErr = cmdutil.ServicesForClient(client).Comment().Delete(ctx, args[0], args[1])
				return delErr
			}); err != nil {
				return err
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.comment.delete", map[string]any{
				"comment_id": args[1],
				"deleted":    true,
			}, resp)
		},
	}
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm destructive delete under `--no-input` / non-TTY")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without calling Jira")
	return cmd
}
