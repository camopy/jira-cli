package issue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/huh"
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/boardscope"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	stdininput "github.com/matcra587/jira-cli/internal/cli/stdin"
	"github.com/matcra587/jira-cli/internal/config"
	editorpkg "github.com/matcra587/jira-cli/internal/editor"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

// NewCommand returns the `issue` verb and all its sub-commands.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("issue", "Work with Jira issues", "resources")
	cmd.AddCommand(issueListCommand())
	cmd.AddCommand(issueMineCommand())
	cmd.AddCommand(issueViewCommand())
	cmd.AddCommand(issueCreateCommand())
	cmd.AddCommand(issueEditCommand())
	cmd.AddCommand(issueTransitionCommand())
	cmd.AddCommand(issueCommentGroup())
	cmd.AddCommand(IssueAttachmentCommand())
	cmd.AddCommand(issueLinkSubCommand())
	cmd.AddCommand(issueWebLinkCommand())
	for _, mk := range WatcherCommands {
		cmd.AddCommand(mk())
	}
	cmd.AddCommand(destructiveIssueCommand("clone", "Clone an issue"))
	cmd.AddCommand(destructiveIssueCommand("move", "Move an issue"))
	cmd.AddCommand(destructiveIssueCommand("delete", "Delete an issue"))
	return cmd
}

// issueMineCommand is a thin shorthand for `jira issue list --assignee me`.
// Shares the same runner as `issue list` so any future change to the list
// path (caching, output shape, …) propagates without diverging.
func issueMineCommand() *cobra.Command {
	var opts issueListOptions
	cmd := &cobra.Command{
		Use:   "mine",
		Short: `List issues assigned to you (alias for "issue list --assignee me")`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.builder.Assignee = "me"
			return runIssueList(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.detail, "detail", false, "Fetch full issue records")
	cmd.Flags().StringVar(&opts.jqlQuery, "jql", "", "Add custom JQL clauses (combined with assignee = currentUser())")
	cmd.Flags().BoolVar(&opts.asJQL, "as-jql", false, "Print the built JQL without calling Jira")
	cmd.Flags().StringSliceVar(&opts.builder.Statuses, "status", nil, "Restrict by status name")
	cmd.Flags().StringSliceVar(&opts.builder.Projects, "project", nil, "Restrict by project key")
	cmdutil.ExtendFlag(cmd.Flags(), "detail", clib.FlagExtra{Group: "Output"})
	cmdutil.ExtendFlag(cmd.Flags(), "jql", clib.FlagExtra{Group: "Filters", Placeholder: "JQL"})
	cmdutil.ExtendFlag(cmd.Flags(), "as-jql", clib.FlagExtra{Group: "Output"})
	cmdutil.ExtendFlag(cmd.Flags(), "status", clib.FlagExtra{Group: "Filters", Placeholder: "NAME"})
	cmdutil.ExtendFlag(cmd.Flags(), "project", clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheproject,comma"})
	return cmd
}

func issueViewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "view KEY",
		Short: "View issue details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				issue, resp, err := cmdutil.IssueService(client).Get(cmd.Context(), args[0], &jira.IssueGetOptions{Expand: []string{"renderedFields", "names", "schema", "transitions", "operations"}})
				if err != nil {
					return err
				}
				// ADF render-loss warnings describe what the HUMAN
				// renderer drops when it flattens ADF to Markdown. The
				// json/compact envelope carries the full ADF in
				// data.issue, so nothing is lost there — emitting the
				// warning on a machine path would be false.
				// Scope the scan to the human output mode only.
				var warnings []adf.Warning
				if cmdutil.UsePlainOutput(cmd) {
					warnings = collectIssueLossyWarnings(issue)
				}
				return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.view", map[string]any{"issue": issue}, resp, warnings)
			}
			return cmdutil.WriteEnvelope(cmd, "issue.view", map[string]any{"issue": map[string]any{"key": args[0]}})
		},
	}
}

// collectIssueLossyWarnings inspects an *jira.Issue for ADF surfaces
// (description, embedded comment bodies) and returns a structured
// warning per surface that contained at least one construct the
// ADF→Markdown renderer can't fully express. The warning shape mirrors
// the comment-list lossy-warning contract: Field identifies the source
// (`description` or `comment:<id>`), NodeType is the dropped node
// name, and Message enumerates the construct list. Multiple lossy
// constructs on a single source produce multiple warnings (one per
// construct) so they fit the existing flat cli.Warning envelope shape;
// consumers that want a per-source rollup can group on Field.
func collectIssueLossyWarnings(issue *jira.Issue) []adf.Warning {
	if issue == nil {
		return nil
	}
	var warnings []adf.Warning
	if issue.Fields != nil && issue.Fields.Description != nil {
		res := adf.ToMarkdownLossy(*issue.Fields.Description)
		for _, c := range res.LossyConstructs {
			warnings = append(warnings, adf.Warning{
				Type:     "adf_lossy_render",
				Message:  fmt.Sprintf("description ADF construct %q dropped during Markdown render; render in --output=json for full ADF fidelity", c),
				Field:    "description",
				NodeType: c,
				Lossy:    true,
			})
		}
	}
	for _, comment := range issue.Comments {
		if comment == nil || comment.Body == nil {
			continue
		}
		res := adf.ToMarkdownLossy(*comment.Body)
		if len(res.LossyConstructs) == 0 {
			continue
		}
		commentID := ""
		if comment.ID != nil {
			commentID = *comment.ID
		}
		field := "comment"
		if commentID != "" {
			field = "comment:" + commentID
		}
		for _, c := range res.LossyConstructs {
			warnings = append(warnings, adf.Warning{
				Type:     "adf_lossy_render",
				Message:  fmt.Sprintf("comment %s ADF construct %q dropped during Markdown render; render in --output=json for full ADF fidelity", commentID, c),
				Field:    field,
				NodeType: c,
				Lossy:    true,
			})
		}
	}
	return warnings
}

// issueListOptions captures every input the issue-list runner needs.
// Both `issue list` and `issue mine` populate it from their flags.
type issueListOptions struct {
	builder  jql.BuildOptions
	jqlQuery string
	detail   bool
	asJQL    bool
}

// runIssueList is the shared body for `issue list` and `issue mine`. It
// applies profile defaults, builds the JQL, optionally short-circuits with
// --as-jql, then either calls Jira or returns an empty envelope when no
// client is configured. Output flows through the same `issue.list` envelope
// shape so consumers can't tell which command emitted it.
func runIssueList(cmd *cobra.Command, opts issueListOptions) error {
	scope, precedence, scopeErr := boardscope.FromFlags(cmd)
	if scopeErr != nil {
		return scopeErr
	}
	//  default_board wins exclusively over default_project on
	// commands that consume --board. When the board scope is active,
	// the builder must NOT inherit default_project — the board's
	// project keys are the sole project clause.
	scopeActive := len(scope.Board.ProjectKeys) > 0
	if opts.asJQL {
		// --as-jql must not require a credential — it never calls Jira.
		// boardscope.FromFlags is cache-only (no client probe), and we
		// load the profile directly here instead of cmdutil.JiraClientForCommand
		// so the secret backend (e.g. 1Password) stays untouched.
		cfg, cfgErr := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
		if cfgErr != nil {
			return cfgErr
		}
		profile := cmdutil.ActiveProfile(cmd, cfg)
		builder := opts.builder
		if !scopeActive && strings.TrimSpace(opts.jqlQuery) == "" {
			builder = issueListBuilderWithProfileDefaults(builder, profile)
		}
		query, err := jql.IssueList(opts.jqlQuery, builder)
		if err != nil {
			return err
		}
		query = boardscope.ApplyClauseToJQL(query, scope)
		return cmdutil.WriteEnvelope(cmd, "issue.list.jql", boardScopedListData(cmd, []map[string]any{}, opts.detail, query, scope, precedence))
	}
	client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	builder := opts.builder
	if !scopeActive && strings.TrimSpace(opts.jqlQuery) == "" {
		builder = issueListBuilderWithProfileDefaults(builder, profile)
	}
	query, err := jql.IssueList(opts.jqlQuery, builder)
	if err != nil {
		return err
	}
	query = boardscope.ApplyClauseToJQL(query, scope)
	if !ok {
		return cmdutil.WriteEnvelope(cmd, "issue.list", boardScopedListData(cmd, []map[string]any{}, opts.detail, query, scope, precedence))
	}
	service := cmdutil.IssueService(client)
	fields := jira.DefaultIssueListFields()
	var expand []string
	if opts.detail {
		fields = []string{"*all"}
		expand = issueListDetailExpand()
	}
	issues, resp, err := service.List(cmd.Context(), &jira.IssueListOptions{
		ListOptions: jira.ListOptions{MaxResults: 50},
		JQL:         query,
		Fields:      fields,
		Expand:      expand,
	})
	if err != nil {
		return err
	}
	issueData := cmdutil.IssueOutput(issues, opts.detail)
	return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.list", boardScopedListData(cmd, issueData, opts.detail, query, scope, precedence), resp)
}

// boardScopedListData extends IssueListOutputData with the new envelope
// fields per contracts/envelope-shapes.md > issue list --board.
func boardScopedListData(cmd *cobra.Command, issues any, detail bool, query string, scope jira.BoardScope, precedence string) map[string]any {
	data := IssueListOutputData(cmd, issues, detail, query)
	data["jql"] = query
	data["precedence"] = precedence
	data["board_scope"] = boardscope.EnvelopeData(scope)
	return data
}

// issueListCommand has been relocated to cmd/jira/issue_list.go per the
// `issue_<verb>.go` convention. The runner (runIssueList) and helpers
// (issueListBuilderWithProfileDefaults, IssueListOutputData) remain in
// this file because they are shared with `issue mine`.

func issueListBuilderWithProfileDefaults(builder jql.BuildOptions, profile config.Profile) jql.BuildOptions {
	if len(jql.CompactStrings(builder.Projects)) == 0 && profile.DefaultProject != "" {
		builder.Projects = []string{profile.DefaultProject}
	}
	return builder
}

func IssueListOutputData(cmd *cobra.Command, issues any, detail bool, query string) map[string]any {
	data := map[string]any{"issues": issues, "detail": detail}
	if debug, _ := cmd.Root().PersistentFlags().GetBool("debug"); debug {
		data["jql"] = query
	}
	return data
}

func issueListDetailExpand() []string {
	return []string{"renderedFields", "names", "schema", "transitions", "operations", "changelog"}
}

func issueCreateCommand() *cobra.Command {
	var dryRun bool
	var summary, jsonInput, assignee string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			payload := map[string]any{"summary": summary}
			if jsonInput != "" {
				if err := cmdutil.ReadJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			// Resolve the profile WITHOUT building a client: --assignee me,
			// --no-input validation, and the dry-run preview only need
			// profile metadata. Credentials are resolved later, at the
			// live-submit boundary.
			profile, err := cmdutil.ProfileForCommand(cmd)
			if err != nil {
				return err
			}
			// --assignee shortcut: feeds the spec's assignee_account_id input
			// when "me" / a literal account-id is supplied. "none" clears it.
			if v := strings.TrimSpace(assignee); v != "" {
				switch strings.ToLower(v) {
				case "none", "unassigned":
					delete(payload, "assignee_account_id")
				case "me", "@me":
					if profile.AccountID != "" {
						payload["assignee_account_id"] = profile.AccountID
					}
				default:
					payload["assignee_account_id"] = v
				}
			}
			if noInput {
				if err := validateIssueCreateRequired(payload, profile); err != nil {
					return err
				}
			}
			// Fill project / issue type from profile defaults BEFORE
			// normalizing aliases so a default-only create still carries
			// the target into the wire payload.
			if cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["project_key"])) == "" && profile.DefaultProject != "" {
				payload["project_key"] = profile.DefaultProject
			}
			if cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["issue_type"])) == "" && profile.DefaultIssueType != "" {
				payload["issue_type"] = profile.DefaultIssueType
			}
			// Normalize the CLI create aliases (project_key / issue_type /
			// assignee_account_id) to the Jira wire field ids
			// (project / issuetype / assignee) BEFORE the pipeline runs.
			// Screen validation keys on the wire ids; an un-normalized
			// alias would be flagged off-screen even for a default
			// create. A conflict (alias and wire key both set) is fatal.
			projectForSchema := cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["project_key"]), profile.DefaultProject)
			issueTypeForSchema := cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["issue_type"]), profile.DefaultIssueType)
			normalizedPayload, normErr := pipeline.NormalizeCreateAliasesChecked(payload)
			if normErr != nil {
				return normErr
			}
			payload = normalizedPayload
			// The issue description is the primary ADF document for
			// this mutation. Whether it arrived as `description_markdown`
			// or as a raw ADF `description`, it is pulled out of the
			// payload here and fed to the pipeline as ADFDoc so stage 2
			// (ValidateDoc + ApplyCompatibility) runs on it BEFORE
			// submission — never as a post-pipeline conversion that
			// skips validation. The post-pipeline SubmitADF is the only
			// description that reaches the wire.
			descriptionDoc, descriptionPresent, descMarkdownWarnings, descErr := extractDescriptionDoc(payload)
			if descErr != nil {
				return descErr
			}
			// Extract any remaining ADF-shaped subfields (e.g.
			// `environment`, `customfield_NNNN` ADF) so stage 2 validates
			// them. `description` was already removed above, so it is
			// not double-validated here. These named docs are
			// validate-only: with no per-field screen schema we cannot
			// know their compatibility envelope, so ApplyCompatibility is
			// not run on them — the same treatment the primary
			// description receives (see extractDescriptionDoc).
			namedADF, adfParseErr := extractNamedADFDocs(payload)
			if adfParseErr != nil {
				return adfParseErr
			}

			// Route through the 5-stage validation pipeline before
			// submission. For a live create we resolve the client up
			// front and attach a screen-schema fetcher so stage 3
			// validates the payload against the project / issue-type
			// create screen. A dry-run preview runs without credentials,
			// so it has no fetcher: stages 1+2+4 still run; stage 3 is a
			// no-op when no schema is reachable.
			var client *jira.Client
			if !dryRun {
				var ok bool
				var err error
				client, profile, ok, err = cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("jira base URL is required for issue.create")
				}
			}
			pipeIn := pipeline.MutationInput{
				Mode:             cmdutil.ADFModeFor(cmd, true),
				Fields:           payload,
				DryRun:           dryRun,
				NamedADFDocs:     namedADF,
				MarkdownWarnings: descMarkdownWarnings,
			}
			if client != nil && !cmdutil.ReadOnlyEnabled(cmd) {
				// Skip the schema fetch in read-only mode: the client
				// refuses the create itself, so resolving its screen is
				// wasted work and would emit a stray read request.
				pipeIn.SchemaFetcher = newScreenSchemaFetcher(
					cmd.Context(), cmdutil.ServicesForClient(client).Project(0),
					cmdutil.ProfileForEnvelope(cmd), projectForSchema, issueTypeForSchema,
				)
			}
			if descriptionPresent {
				// FieldCompatibility is left at its zero value with
				// InlineCardSupported=true: a Jira Cloud `description`
				// field accepts inlineCard, so compatibility degradation
				// would be wrong here. ApplyCompatibility still walks the
				// doc; it just has nothing to degrade.
				pipeIn.ADFDoc = &descriptionDoc
				pipeIn.FieldCompat = &adf.FieldCompatibility{Field: "description", InlineCardSupported: true}
			}
			pipeOut := pipeline.RunMutation(pipeIn)
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Every path past validation uses the pipeline's
			// post-validation, post-encoding output — never the raw
			// pre-pipeline payload. SubmitFields is the only map allowed
			// downstream.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			// The validated, compatibility-applied description from the
			// pipeline replaces whatever the payload carried — markdown
			// and raw ADF are now handled identically.
			if descriptionPresent && pipeOut.SubmitADF != nil {
				submitFields["description"] = *pipeOut.SubmitADF
			}
			if dryRun {
				preview, err := issueCreatePreview(submitFields, profile)
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.create", map[string]any{
					"preview": preview,
					"dry_run": true,
				}, pipeOut.Warnings)
			}
			// submitFields already carries project / issuetype / assignee
			// in their Jira wire shapes — alias normalization ran before
			// the pipeline. IssueCreateRequest forwards the fields map
			// verbatim; Project / IssueType are left empty so it does not
			// re-wrap a value that is already wire-shaped.
			req := &jira.IssueCreateRequest{
				Summary: cmdutil.StringFromAny(submitFields["summary"]),
				Fields:  submitFields,
			}
			issue, resp, err := cmdutil.IssueService(client).Create(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.create", map[string]any{"issue": issue, "dry_run": false}, resp, pipeOut.Warnings)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().StringVar(&summary, "summary", "", "Issue summary")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read issue create payload from JSON file")
	cmd.Flags().StringVar(&assignee, "assignee", "", `Assign on creation: "me" or a Jira account ID`)
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendFlag(cmd.Flags(), "summary", clib.FlagExtra{Group: "Fields", Placeholder: "TEXT"})
	cmdutil.ExtendFileFlag(cmd.Flags(), "json-input", "Input", "FILE")
	cmdutil.ExtendFlag(cmd.Flags(), "assignee", clib.FlagExtra{Group: "Fields", Placeholder: "USER"})
	return cmd
}

// cmdutil.ConfiguredEditorFor returns the editor.Resolve(...) "configured"
// argument for the active invocation: the active profile's Editor
// field if set, otherwise the global Config.Editor. The resolver in
// internal/editor layers $JIRA_EDITOR / $EDITOR / $VISUAL / "vi" on
// top of whatever this returns.
// resolveAssigneeField turns an --assignee flag value into the Jira `assignee`
// field shape. Accepted values:
//   - ""                         → no change (set ok=false)
//   - "me" / "@me"               → profile.AccountID (Cloud) or profile.Username (Server/DC)
//   - "none" / "unassigned"      → nil (clears assignee)
//   - "<accountId>"              → {accountId: "..."}
//
// Returns (value, set, error). set=false with err=nil means no flag was given
// (no field change). err is non-nil when input was supplied but couldn't be
// resolved — the caller MUST surface this rather than silently dropping it,
// because Jira Cloud ignores email-based assignment without complaint.
func resolveAssigneeField(input string, profile config.Profile) (any, bool, error) {
	v := strings.TrimSpace(input)
	if v == "" {
		return nil, false, nil
	}
	switch strings.ToLower(v) {
	case "none", "unassigned":
		return nil, true, nil
	case "me", "@me":
		switch {
		case profile.AccountID != "":
			return map[string]string{"accountId": profile.AccountID}, true, nil
		case profile.Username != "":
			return map[string]string{"name": profile.Username}, true, nil
		}
		return nil, false, fmt.Errorf("--assignee me requires profile.account_id (Cloud) or profile.username (Server/DC); run `jira auth whoami --save` to populate it")
	default:
		return map[string]string{"accountId": v}, true, nil
	}
}

// validateIssueCreateRequired enforces the spec rule "headless write commands
// require complete input via --no-input + --json-input". It checks that
// project_key, issue_type, and summary are derivable from the supplied JSON
// payload or profile defaults, otherwise returns a validation error.
func validateIssueCreateRequired(payload map[string]any, profile config.Profile) error {
	var missing []string
	if cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["summary"])) == "" {
		missing = append(missing, "summary")
	}
	if cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["project_key"]), profile.DefaultProject) == "" {
		missing = append(missing, "project_key")
	}
	if cmdutil.FirstNonEmpty(cmdutil.StringFromAny(payload["issue_type"]), profile.DefaultIssueType) == "" {
		missing = append(missing, "issue_type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("--no-input requires complete input: missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// issueCreatePreview builds the dry-run preview shape per command-schemas.md.
// The supplied fields map is the pipeline's post-validation SubmitFields:
// the create aliases have been normalized to the Jira wire field ids
// (project / issuetype / assignee) and the description (whether it
// arrived as markdown or raw ADF) has been converted, validated, and
// compatibility-applied under the bare `description` key. Profile
// defaults fill project / issue type when the payload omits them.
func issueCreatePreview(fields map[string]any, profile config.Profile) (map[string]any, error) {
	preview := map[string]any{
		"project_key": cmdutil.FirstNonEmpty(cmdutil.WireObjectString(fields["project"], "key"), profile.DefaultProject),
		"issue_type":  cmdutil.FirstNonEmpty(cmdutil.WireObjectString(fields["issuetype"], "name"), profile.DefaultIssueType),
		"summary":     cmdutil.StringFromAny(fields["summary"]),
	}
	// description in SubmitFields is the validated ADF document. Surface
	// it under description_adf so the preview names the wire shape.
	if doc, ok := fields["description"]; ok && doc != nil {
		preview["description_adf"] = doc
	}
	if acct := cmdutil.WireObjectString(fields["assignee"], "accountId"); acct != "" {
		preview["assignee_account_id"] = acct
	}
	for _, key := range []string{"priority", "labels", "components", "epic_key", "custom_fields"} {
		if v, ok := fields[key]; ok {
			preview[key] = v
		}
	}
	return preview, nil
}

// extractDescriptionDoc pulls the issue description out of an issue-create
// payload as a single adf.Document, removing the source key(s) from the
// map so the description is processed exactly once — as the pipeline's
// primary ADFDoc, not as an opaque named subfield.
//
// Two input shapes are accepted, in priority order:
//   - `description_markdown`: a Markdown string, converted via
//     FromMarkdownLossy. Conversion warnings are returned so the pipeline
//     can abort (strict) or surface them (best-effort) on content loss.
//   - `description`: a raw ADF document object.
//
// present is false when neither key is set. When present, the returned
// doc is what the pipeline validates; the caller writes the pipeline's
// SubmitADF back into the fields map under `description`.
func extractDescriptionDoc(payload map[string]any) (doc adf.Document, present bool, warnings []adf.Warning, err error) {
	if md := cmdutil.StringFromAny(payload["description_markdown"]); md != "" {
		delete(payload, "description_markdown")
		delete(payload, "description")
		converted, convWarnings, cerr := adf.FromMarkdownLossy(md)
		if cerr != nil {
			return adf.Document{}, false, nil, fmt.Errorf("convert description_markdown to ADF: %w", cerr)
		}
		return converted, true, convWarnings, nil
	}
	raw, ok := payload["description"]
	if !ok || raw == nil {
		return adf.Document{}, false, nil, nil
	}
	delete(payload, "description")
	encoded, merr := json.Marshal(raw)
	if merr != nil {
		return adf.Document{}, false, nil, fmt.Errorf("marshal description for ADF validation: %w", merr)
	}
	parsed, _, perr := adf.Parse(encoded)
	if perr != nil {
		return adf.Document{}, false, nil, fmt.Errorf("description: %w", perr)
	}
	return parsed, true, nil, nil
}

func issueEditCommand() *cobra.Command {
	var dryRun bool
	var jsonInput, summary, assignee string
	cmd := &cobra.Command{
		Use:   "edit KEY",
		Short: "Edit an issue",
		Long: `Edit a Jira issue.

With no field flags, opens the configured external editor on the issue
description (kubectl-style). Use --summary / --assignee / --json-input
for headless or single-field edits.

In headless mode (--no-input), at least one field flag MUST be provided
— there is no editor to open and silent no-ops are validation errors.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			payload := map[string]any{"fields": map[string]any{}}
			if jsonInput != "" {
				if err := cmdutil.ReadJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			fields, ok := payload["fields"].(map[string]any)
			if !ok {
				return fmt.Errorf("issue edit JSON input must contain a fields object")
			}
			// --summary / --assignee shortcuts, applied on top of any --json-input.
			// Resolve the profile only — building a client here would
			// resolve credentials even on a dry-run or editor-only path.
			profile, err := cmdutil.ProfileForCommand(cmd)
			if err != nil {
				return err
			}
			if v := strings.TrimSpace(summary); v != "" {
				fields["summary"] = v
			}
			if v, set, err := resolveAssigneeField(assignee, profile); err != nil {
				return err
			} else if set {
				fields["assignee"] = v
			}
			// kubectl-style default: bare `jira issue edit KEY` (no field
			// flags, no --json-input) opens the configured external editor
			// on the issue description. The editor reads keystrokes from
			// stdin, so the gate checks stdin specifically — piping stdout
			// (`jira issue edit KEY | tee out.json`) is a legitimate human
			// workflow that must NOT trip the refusal. det.Agent covers
			// LLM-agent harnesses regardless of stdin shape.
			if len(fields) == 0 {
				if noInput {
					return fmt.Errorf("validation: no fields specified for issue edit; provide --summary, --assignee, or --json-input")
				}
				det := cmdutil.DetectorFromContext(cmd)
				if det.Agent || !stdininput.IsTerminal() {
					return fmt.Errorf("validation: issue edit requires an interactive terminal for the editor flow; in agent or non-TTY context, provide --summary, --assignee, or --json-input")
				}
				return issueEditWithEditor(cmd, args[0], dryRun)
			}
			// Any ADF-shaped value in the fields object (e.g. a raw
			// `description` document supplied via --json-input) MUST be
			// validated by stage 2 before submission — otherwise garbage
			// nested ADF would only be checked structurally by the
			// customfield encoder, which does not enforce ADF rules.
			namedADF, adfParseErr := extractNamedADFDocs(fields)
			if adfParseErr != nil {
				return adfParseErr
			}
			// For a live edit, resolve the client up front and attach an
			// edit-screen schema fetcher (editmeta) so stage 3 validates
			// the fields against the issue's edit screen. A dry-run runs
			// without credentials and therefore without a fetcher.
			var editClient *jira.Client
			if !dryRun {
				var hasClient bool
				editClient, _, hasClient, err = cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if !hasClient {
					return fmt.Errorf("jira base URL is required for issue.edit")
				}
			}
			// Thread the mutation through the 5-stage pipeline.
			editIn := pipeline.MutationInput{
				Mode:         cmdutil.ADFModeFor(cmd, true),
				Fields:       fields,
				NamedADFDocs: namedADF,
				DryRun:       dryRun,
			}
			if editClient != nil && !cmdutil.ReadOnlyEnabled(cmd) {
				editIn.SchemaFetcher = newEditScreenSchemaFetcher(
					cmd.Context(), cmdutil.ServicesForClient(editClient).Project(0),
					cmdutil.ProfileForEnvelope(cmd), args[0],
				)
			}
			pipeOut := pipeline.RunMutation(editIn)
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Submit and preview the validated SubmitFields, not
			// the raw pre-pipeline fields map.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			if !dryRun {
				issue, resp, err := cmdutil.IssueService(editClient).Update(cmd.Context(), args[0], &jira.IssueUpdateRequest{Fields: submitFields})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.edit", map[string]any{
					"issue":   args[0],
					"result":  issue,
					"dry_run": false,
					"fields":  submitFields,
				}, resp, pipeOut.Warnings)
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.edit", map[string]any{
				"issue":   args[0],
				"dry_run": true,
				"fields":  submitFields,
			}, pipeOut.Warnings)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read issue edit payload from JSON file")
	cmd.Flags().StringVar(&summary, "summary", "", "Replace the issue summary")
	cmd.Flags().StringVar(&assignee, "assignee", "", `Set assignee: "me", "none"/"unassigned", or a Jira account ID`)
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendFileFlag(cmd.Flags(), "json-input", "Input", "FILE")
	cmdutil.ExtendFlag(cmd.Flags(), "summary", clib.FlagExtra{Group: "Fields", Placeholder: "TEXT"})
	cmdutil.ExtendFlag(cmd.Flags(), "assignee", clib.FlagExtra{Group: "Fields", Placeholder: "USER"})
	return cmd
}

func issueEditWithEditor(cmd *cobra.Command, key string, dryRun bool) error {
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.edit --edit")
	}
	issueService := cmdutil.IssueService(client)
	issue, _, err := issueService.Get(cmd.Context(), key, &jira.IssueGetOptions{})
	if err != nil {
		return err
	}
	doc := adf.Document{Type: "doc", Version: 1}
	if issue != nil && issue.Fields != nil && issue.Fields.Description != nil {
		doc = *issue.Fields.Description
	}
	// Route the edit through the opaque-preserving round-trip. Blocks
	// with no faithful Markdown representation — panels, tables,
	// inlineCards, mentions — are carried through the editor buffer as
	// protected opaque fences and reconstituted byte-for-byte, so a
	// no-op save can no longer erase rich Jira content. A plain
	// adf.ToMarkdown render would have dropped them before the user
	// ever saw the buffer.
	updatedDoc, editWarnings, err := editorpkg.RoundTripADF(cmd.Context(), editorpkg.RoundTripADFOptions{
		IssueKey:  key,
		FieldName: "description",
		Document:  doc,
		EditCmd:   cmdutil.ConfiguredEditorFor(cmd),
	})
	if err != nil {
		return err
	}
	// External-editor edits ARE mutations and MUST run through the
	// pipeline. The edited description is the only field, and it is an
	// ADF document — route it solely as ADFDoc so stage 2 (ValidateDoc
	// + ApplyCompatibility) owns it. It is deliberately NOT also placed
	// in Fields: a single field must travel one pipeline channel, so
	// there is no last-write-wins reconciliation between SubmitFields
	// and SubmitADF. Stage 4 (customfield encoding) has nothing to do
	// for an empty Fields map.
	//
	// The round-trip's warnings — lossy Markdown conversions on the
	// edited text plus opaque-preservation notices — travel as
	// MarkdownWarnings so strict mode aborts on genuine content loss
	// before submission.
	pipeOut := pipeline.RunMutation(pipeline.MutationInput{
		Mode:             cmdutil.ADFModeFor(cmd, true),
		ADFDoc:           &updatedDoc,
		FieldCompat:      &adf.FieldCompatibility{Field: "description", InlineCardSupported: true},
		MarkdownWarnings: editWarnings,
		DryRun:           dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit the validated, compatibility-applied SubmitADF — not
	// the pre-pipeline edit. description is the sole field on the wire.
	submitFields := map[string]any{}
	if pipeOut.SubmitADF != nil {
		submitFields["description"] = *pipeOut.SubmitADF
	}
	if dryRun {
		return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.edit", map[string]any{
			"issue":   key,
			"dry_run": true,
			"fields":  submitFields,
		}, pipeOut.Warnings)
	}
	updatedIssue, resp, err := issueService.Update(cmd.Context(), key, &jira.IssueUpdateRequest{Fields: submitFields})
	if err != nil {
		return err
	}
	return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.edit", map[string]any{
		"issue":   key,
		"result":  updatedIssue,
		"dry_run": false,
		"fields":  submitFields,
	}, resp, pipeOut.Warnings)
}

func issueTransitionCommand() *cobra.Command {
	var dryRun bool
	var transitionID string
	returnCmd := &cobra.Command{
		Use:   "transition KEY",
		Short: "Transition an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A workflow transition is a Jira mutation and MUST run
			// through the 5-stage pipeline. No fields/ADF doc to
			// validate today (transitions don't carry payload here),
			// but the parse + dry-run gating still apply.
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:   cmdutil.ADFModeFor(cmd, true),
				DryRun: dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			if dryRun {
				return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "transition": transitionID, "dry_run": dryRun}, pipeOut.Warnings)
			}
			if transitionID == "" {
				// List available transitions — this is a READ, not a
				// mutation; it returns successor IDs the caller chooses
				// from. Skip the warnings helper since pipeline only
				// runs to satisfy stage gating consistency.
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					transitions, resp, err := cmdutil.IssueService(client).Transitions(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.transitions", map[string]any{"issue": args[0], "transitions": transitions}, resp, pipeOut.Warnings)
				}
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok && transitionID != "" {
				resp, err := cmdutil.IssueService(client).Transition(cmd.Context(), args[0], &jira.TransitionRequest{ID: transitionID})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "transition": transitionID, "dry_run": false}, resp, pipeOut.Warnings)
			}
			if !dryRun && transitionID != "" {
				return fmt.Errorf("jira base URL is required for issue.transition")
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "dry_run": dryRun}, pipeOut.Warnings)
		},
	}
	returnCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	returnCmd.Flags().StringVar(&transitionID, "transition", "", "Transition ID to execute")
	cmdutil.ExtendDryRunFlag(returnCmd.Flags())
	cmdutil.ExtendFlag(returnCmd.Flags(), "transition", clib.FlagExtra{Group: "Transition", Placeholder: "ID"})
	return returnCmd
}

func destructiveIssueCommand(name, short string) *cobra.Command {
	var dryRun, force, deleteSubtasks bool
	var jsonInput string
	cmd := &cobra.Command{
		Use:   name + " KEY",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			payload := map[string]any{"fields": map[string]any{}}
			if jsonInput != "" {
				if err := cmdutil.ReadJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			// Destructive commands ARE mutations and MUST run through
			// the pipeline. delete has no fields to validate (stages
			// 2-4 no-op); clone/move carry a fields payload that stages
			// 3 (screen schema) and 4 (customfield encoding) validate.
			pipeFields := issueFieldsFromPayload(payload)
			// For a live clone/move, resolve the client up front and
			// attach an edit-screen schema fetcher (editmeta on the
			// source issue) so stage 3 validates the override fields
			// against a real screen. delete carries no fields, and a
			// dry-run runs without credentials — neither gets a fetcher.
			var destructiveClient *jira.Client
			var err error
			if !dryRun && name != "delete" {
				var hasClient bool
				destructiveClient, _, hasClient, err = cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if !hasClient {
					return fmt.Errorf("jira base URL is required for issue.%s", name)
				}
			}
			destructiveIn := pipeline.MutationInput{
				Mode:   cmdutil.ADFModeFor(cmd, true),
				Fields: pipeFields,
				DryRun: dryRun,
			}
			if destructiveClient != nil && !cmdutil.ReadOnlyEnabled(cmd) {
				destructiveIn.SchemaFetcher = newEditScreenSchemaFetcher(
					cmd.Context(), cmdutil.ServicesForClient(destructiveClient).Project(0),
					cmdutil.ProfileForEnvelope(cmd), args[0],
				)
			}
			pipeOut := pipeline.RunMutation(destructiveIn)
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Clone/move submit the validated SubmitFields. delete
			// carries no field payload, so SubmitFields is an empty map.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			if dryRun {
				return cmdutil.WriteEnvelopeWithWarnings(cmd, "issue."+name, map[string]any{"issue": args[0], "payload": map[string]any{"fields": submitFields}, "dry_run": true}, pipeOut.Warnings)
			}
			// Destructive op safety: in TTY mode (a human at the
			// keyboard) require either --force OR an interactive
			// "are you sure?" confirmation. Headless / agent shells
			// MUST pass --force explicitly — the auto-detect refuses
			// to prompt them.
			det := cmdutil.DetectorFromContext(cmd)
			if !force {
				// Non-TTY / agent / --no-input → MUST pass --force.
				// We refuse to prompt headless callers and refuse to
				// proceed without explicit consent.
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("issue %s requires --force in headless / agent / --no-input mode", name)
				}
				// TTY human → huh confirmation prompt.
				if ok, err := confirmDestructive(cmd, name, args[0]); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "issue "+name, nil)
				}
			}
			// clone/move already resolved a client up front to drive the
			// schema fetcher; delete resolves one here.
			client, ok := destructiveClient, destructiveClient != nil
			if client == nil {
				client, _, ok, err = cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
			}
			if ok && !dryRun {
				service := cmdutil.IssueService(client)
				var resp *jira.Response
				var issue *jira.Issue
				switch name {
				case "delete":
					resp, err = service.Delete(cmd.Context(), args[0], &jira.IssueDeleteOptions{DeleteSubtasks: deleteSubtasks})
				case "clone":
					issue, resp, err = service.Clone(cmd.Context(), args[0], &jira.IssueCloneRequest{Fields: submitFields})
				case "move":
					issue, resp, err = service.Move(cmd.Context(), args[0], &jira.IssueMoveRequest{Fields: submitFields})
				}
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "issue."+name, map[string]any{"issue": args[0], "result": issue, "dry_run": false}, resp, pipeOut.Warnings)
			}
			return fmt.Errorf("jira base URL is required for issue.%s", name)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive mutation")
	cmd.Flags().BoolVar(&deleteSubtasks, "delete-subtasks", false, "(delete only) also delete the issue's subtasks (Jira refuses delete otherwise when subtasks exist)")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read mutation payload from JSON file")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendForceFlag(cmd.Flags())
	cmdutil.ExtendFlag(cmd.Flags(), "delete-subtasks", clib.FlagExtra{Group: "Safety"})
	cmdutil.ExtendFileFlag(cmd.Flags(), "json-input", "Input", "FILE")
	return cmd
}

// confirmDestructive prompts the user via huh for an interactive
// yes/no confirmation before executing a destructive op
// (delete/clone/move). Only invoked in TTY mode; non-TTY callers
// MUST pass --force and are rejected by the caller before reaching
// this function under stdin discipline.
//
// The prompt runs under the command context so a SIGINT or an elapsed
// --timeout cancels the prompt instead of leaving it blocked on the
// terminal. A canceled prompt returns a typed *cli.PromptError so the
// envelope keeps the cancellation identity; a declined confirmation
// returns (false, nil) and the caller turns it into an abort.
func confirmDestructive(cmd *cobra.Command, action, key string) (bool, error) {
	confirmed := false
	confirm := huh.NewConfirm().
		Title(fmt.Sprintf("About to %s %s", action, key)).
		Description("This is destructive. Continue?").
		Affirmative("Yes, " + action).
		Negative("Cancel").
		Value(&confirmed)
	form := huh.NewForm(huh.NewGroup(confirm))
	if err := form.RunWithContext(cmd.Context()); err != nil {
		switch {
		case errors.Is(err, huh.ErrUserAborted):
			// Esc / Ctrl-C inside the form is a decline, not a fault.
			return false, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return false, cli.NewPromptError(cli.PromptCanceled, action+" confirmation", err)
		default:
			return false, cli.NewPromptError(cli.PromptUnavailable, action+" confirmation", err)
		}
	}
	return confirmed, nil
}

// issueWebLinkCommand wires `jira issue weblink KEY --url URL --title T`.
// Goes through POST /rest/api/3/issue/{key}/remotelink — Jira's
// "Web links" feature, separate from issue-to-issue links.
func issueWebLinkCommand() *cobra.Command {
	var url, title string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "weblink KEY",
		Short: "Attach a web link (URL + title) to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" {
				return fmt.Errorf("validation: --url is required")
			}
			// Local URL syntax validation runs in BOTH dry-run and live
			// mode. dry-run is a local preview: it checks the URL parses
			// and carries an absolute http/https scheme, but it cannot
			// and does not verify the target is reachable.
			if err := validateWebLinkURL(url); err != nil {
				return err
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "issue.weblink", map[string]any{
					"issue": args[0], "url": url, "title": title, "dry_run": true,
					// Be explicit that dry-run did NOT contact the
					// target URL — only its syntax was checked locally.
					"url_remote_checked": false,
				})
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.weblink")
			}
			resp, err := cmdutil.IssueService(client).AddRemoteLink(cmd.Context(), args[0], &jira.RemoteLinkRequest{
				URL: url, Title: title,
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.weblink", map[string]any{
				"issue": args[0], "url": url, "title": title, "dry_run": false,
			}, resp)
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "Web link target URL (required)")
	cmd.Flags().StringVar(&title, "title", "", "Display title for the link")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without creating the link")
	cmdutil.ExtendFlag(cmd.Flags(), "url", clib.FlagExtra{Group: "Link", Placeholder: "URL", Hint: "url"})
	cmdutil.ExtendFlag(cmd.Flags(), "title", clib.FlagExtra{Group: "Link", Placeholder: "TEXT"})
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}

// validateWebLinkURL performs the local, offline syntax check the
// weblink dry-run promises: the value must parse and carry an absolute
// http/https URL. It deliberately does NOT fetch the target — dry-run
// is local preview only, and reachability is not its job.
func validateWebLinkURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("validation: --url %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("validation: --url %q must be an absolute http or https URL", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("validation: --url %q is missing a host", raw)
	}
	return nil
}

func issueFieldsFromPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	if rawFields, ok := payload["fields"]; ok {
		if fields, ok := rawFields.(map[string]any); ok {
			return cmdutil.CopyAnyMap(fields)
		}
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "dry_run" {
			continue
		}
		out[key] = value
	}
	return out
}

func extractNamedADFDocs(payload map[string]any) (map[string]adf.Document, error) {
	var named map[string]adf.Document
	for k, v := range payload {
		if !looksLikeADFDoc(v) {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal %s for ADF validation: %w", k, err)
		}
		doc, _, err := adf.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		if named == nil {
			named = make(map[string]adf.Document)
		}
		named[k] = doc
	}
	return named, nil
}

// looksLikeADFDoc reports whether v is an object whose top-level shape is an
// ADF root document — type field equals "doc" and a version field is present.
// Cheap shape gate before the full marshal+parse path; avoids parsing every
// nested object (e.g. assignee={accountId:...}, project={key:...}) as ADF.
func looksLikeADFDoc(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	t, _ := m["type"].(string)
	if t != "doc" {
		return false
	}
	_, hasVersion := m["version"]
	return hasVersion
}
