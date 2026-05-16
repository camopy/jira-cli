package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira"
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
		Use:         "comment KEY",
		Short:       "Manage comments on a Jira issue",
		Long:        "Add, list, edit, or delete comments. With no subcommand, behaves as `comment add KEY ...` for back-compat with the legacy `jira issue comment KEY` invocation.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runCommentAdd(cmd, args[0], addFlags)
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
	cmd := &cobra.Command{
		Use:         "list KEY",
		Short:       "List comments on an issue",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommentList(cmd, args[0], limit, all)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum comments per page")
	cmd.Flags().BoolVar(&all, "all", false, "Walk every page until isLast")
	return cmd
}

// runCommentList fetches `/issue/{key}/comment` and emits the standard
// envelope under --json/--compact, raw Atlassian shape under --raw, or a
// plain table otherwise. With --all, the service-layer drain handles
// pagination and reports rate-limit-during-paginate as partial success.
func runCommentList(cmd *cobra.Command, key string, limit int, all bool) error {
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.list")
	}
	svc := jira.NewCommentService(client)

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
		drained, err := svc.ListAll(cmd.Context(), key, jira.CommentDrainOptions{PageSize: pageSize})
		if err != nil {
			return err
		}
		collected = drained.Comments
		lastResp = drained.LastResp
		pagesFetched = drained.PagesFetched
		rateLimitHit = drained.RateLimitHit
	} else {
		comments, resp, err := svc.List(cmd.Context(), key, &jira.ListCommentsOptions{
			ListOptions: jira.ListOptions{MaxResults: pageSize}, //nolint:gocritic // pagination-exempt: single-page request, no cursor follow-up.
		})
		if err != nil {
			return err
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
	return writeEnvelopeWithCommentWarnings(cmd, "issue.comment.list", data, warnings)
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
// envelope-shapes.md, converting ADF body to Markdown for the default
// (non --raw) path.
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
		m["body"] = adf.ToMarkdown(*c.Body)
	} else {
		m["body"] = ""
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
		// retry_after_seconds is best-effort: the 429 response object
		// isn't surfaced from svc.List right now; default to 0 and let
		// the consumer retry whenever they choose.
		out = append(out, map[string]any{
			"type":                "rate-limit-during-paginate",
			"pages_fetched":       pagesFetched,
			"retry_after_seconds": 0,
		})
	}
	return out
}

// writeEnvelopeWithCommentWarnings emits the envelope with snake-case
// per-comment warnings under warnings[]. We don't reuse
// writeEnvelopeWithWarnings because that helper is shaped around adf.Warning
// (Type/Message/Field/NodeType etc.) — comment-list warnings are a different
// schema.
func writeEnvelopeWithCommentWarnings(cmd *cobra.Command, command string, data any, warnings []map[string]any) error {
	if useCompactOutput(cmd) {
		return cli.WriteCompact(cmd.OutOrStdout(), foldRawWarningsIntoData(data, warnings))
	}
	if usePlainOutput(cmd) {
		return cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, plainOptionsForCommand(cmd)...)
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
	enc.SetIndent("", "  ")
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
	markdown  string
	jsonInput string
	visRole   string
	visGroup  string
	dryRun    bool
}

func commentAddCommand() *cobra.Command {
	flags := commentAddFlags{}
	cmd := &cobra.Command{
		Use:         "add KEY",
		Short:       "Add a comment to an issue",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommentAdd(cmd, args[0], flags)
		},
	}
	registerCommentAddFlags(cmd, &flags)
	return cmd
}

func registerCommentAddFlags(cmd *cobra.Command, flags *commentAddFlags) {
	cmd.Flags().StringVar(&flags.markdown, "body-markdown", "", "Comment body as Markdown (lossy convenience layer)")
	cmd.Flags().StringVar(&flags.jsonInput, "json-input", "", "Comment body as native ADF JSON file (canonical for agents)")
	cmd.Flags().StringVar(&flags.visRole, "visibility-role", "", "Restrict comment to a Jira role (e.g. Developers)")
	cmd.Flags().StringVar(&flags.visGroup, "visibility-group", "", "Restrict comment to a Jira group")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview mutation without submitting")
	// Exactly one body source: a Markdown convenience string or a native
	// ADF JSON file. Declared as Cobra flag metadata so the conflict is
	// rejected before RunE reads either source.
	cmd.MarkFlagsMutuallyExclusive("body-markdown", "json-input")
}

func runCommentAdd(cmd *cobra.Command, key string, flags commentAddFlags) error {
	doc, err := buildCommentBody(cmd, flags.markdown, flags.jsonInput, noInputRequested(cmd))
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
		Mode:   adfModeFor(cmd, true),
		ADFDoc: &doc,
		DryRun: flags.dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit and preview the validated SubmitADF, not the
	// pre-pipeline document. ADFDoc was non-nil above, so the pipeline
	// always sets SubmitADF.
	submitDoc := pipeOut.SubmitADF
	if flags.dryRun {
		return writeEnvelopeWithWarnings(cmd, "issue.comment.add", map[string]any{
			"issue":   key,
			"comment": map[string]any{"body": submitDoc},
			"dry_run": true,
		}, pipeOut.Warnings)
	}
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.add")
	}
	body := &jira.CommentBody{ADF: submitDoc}
	svc := jira.NewCommentService(client)
	var (
		comment *jira.Comment
		resp    *jira.Response
		addErr  error
	)
	if vis.Mode == jira.VisibilityReplace {
		comment, resp, addErr = svc.AddWithVisibility(cmd.Context(), key, body, vis)
	} else {
		comment, resp, addErr = svc.Add(cmd.Context(), key, body)
	}
	if addErr != nil {
		return addErr
	}
	return writeEnvelopeWithResponseAndWarnings(cmd, "issue.comment.add", map[string]any{
		"issue":   key,
		"comment": commentToMap(comment),
		"dry_run": false,
	}, resp, pipeOut.Warnings)
}

// buildCommentBody parses --body-markdown / --json-input into an ADF doc.
// Pre-flight enforces:
//   - the resulting ADF is non-empty
//   - --no-input requires explicit body input
//
// The body-markdown / json-input exclusivity is enforced declaratively
// by cmd.MarkFlagsMutuallyExclusive, so it never reaches here.
func buildCommentBody(cmd *cobra.Command, markdown, jsonInput string, noInput bool) (adf.Document, error) {
	markdownSet := cmd != nil && cmd.Flags().Changed("body-markdown")
	// Empty/missing body: prefer the explicit "body is required" wording so
	// the validation error surfaces consistently. The --no-input rider still
	// appears when the caller passed neither flag *and* opted out of prompts.
	if strings.TrimSpace(markdown) == "" && jsonInput == "" {
		switch {
		case markdownSet:
			return adf.Document{}, fmt.Errorf("validation: comment body is required: --body-markdown is empty")
		case noInput:
			return adf.Document{}, fmt.Errorf("validation: comment body is required: --no-input requires --body-markdown or --json-input")
		default:
			return adf.Document{}, fmt.Errorf("validation: comment body is required (use --body-markdown or --json-input)")
		}
	}
	if jsonInput != "" {
		var payload map[string]any
		if err := readJSONFile(jsonInput, &payload); err != nil {
			return adf.Document{}, err
		}
		if body, ok := payload["body"].(map[string]any); ok {
			payload = body
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return adf.Document{}, err
		}
		parsed, _, err := adf.Parse(raw)
		if err != nil {
			return adf.Document{}, fmt.Errorf("comment --json-input parse: %w", err)
		}
		if len(parsed.Content) == 0 {
			return adf.Document{}, fmt.Errorf("validation: comment body is required: --json-input doc has no content")
		}
		return parsed, nil
	}
	if strings.TrimSpace(markdown) == "" {
		return adf.Document{}, fmt.Errorf("validation: comment body is required: --body-markdown is empty")
	}
	doc, err := adf.FromMarkdown(markdown)
	if err != nil {
		return adf.Document{}, err
	}
	if len(doc.Content) == 0 {
		return adf.Document{}, fmt.Errorf("validation: comment body is required: Markdown produced empty ADF")
	}
	return doc, nil
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
		Use:         "edit KEY COMMENT_ID",
		Short:       "Edit an existing comment",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommentEdit(cmd, args[0], args[1], flags)
		},
	}
	cmd.Flags().StringVar(&flags.markdown, "body-markdown", "", "New body as Markdown")
	cmd.Flags().StringVar(&flags.jsonInput, "json-input", "", "New body as native ADF JSON file")
	cmd.Flags().StringVar(&flags.visRole, "visibility-role", "", "Replace visibility with a Jira role")
	cmd.Flags().StringVar(&flags.visGroup, "visibility-group", "", "Replace visibility with a Jira group")
	cmd.Flags().BoolVar(&flags.visClear, "clear-visibility", false, "Remove any existing visibility restriction")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview without calling Jira")
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
	doc, err := buildCommentBody(cmd, flags.markdown, flags.jsonInput, noInputRequested(cmd))
	if err != nil {
		return err
	}
	pipeOut := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfModeFor(cmd, true),
		ADFDoc: &doc,
		DryRun: flags.dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit and preview the validated SubmitADF. ADFDoc was
	// non-nil above, so the pipeline always sets SubmitADF.
	submitDoc := pipeOut.SubmitADF
	if flags.dryRun {
		return writeEnvelopeWithWarnings(cmd, "issue.comment.edit", map[string]any{
			"issue":             key,
			"comment_id":        commentID,
			"body_adf_summary":  submitDoc,
			"visibility_change": describeVisibilityChange(vis),
			"dry_run":           true,
		}, pipeOut.Warnings)
	}
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.comment.edit")
	}
	comment, resp, err := jira.NewCommentService(client).Edit(cmd.Context(), key, commentID, &jira.CommentBody{ADF: submitDoc}, vis)
	if err != nil {
		return err
	}
	return writeEnvelopeWithResponseAndWarnings(cmd, "issue.comment.edit", map[string]any{
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
		Use:         "delete KEY COMMENT_ID",
		Short:       "Delete a comment",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := noInputRequested(cmd)
			if dryRun {
				return writeEnvelope(cmd, "issue.comment.delete", map[string]any{
					"issue":      args[0],
					"comment_id": args[1],
					"dry_run":    true,
				})
			}
			// Destructive op safety — same shape as `attachment delete`,
			// `link delete`, and `destructiveIssueCommand`. Headless /
			// agent / --no-input MUST pass --force; TTY humans get an
			// interactive confirmation when --force is omitted.
			det := DetectorFromContext(cmd)
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
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.comment.delete")
			}
			resp, err := jira.NewCommentService(client).Delete(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return writeEnvelopeWithResponse(cmd, "issue.comment.delete", map[string]any{
				"comment_id": args[1],
				"deleted":    true,
			}, resp)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive delete under --no-input / non-TTY")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without calling Jira")
	return cmd
}
