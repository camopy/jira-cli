package jql

import (
	"context"
	"encoding/json"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/boardscope"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/spf13/cobra"
)

// NewCommand returns the `jql` command group for building JQL queries.
func NewCommand() *cobra.Command {
	var builder jql.BuildOptions
	cmd := cmdutil.GroupCommand("jql", "Build JQL queries", "resources")
	cmd.Long = "Build and check JQL without leaving the terminal. `jira jql build` assembles " +
		"a query from flags (project, status, assignee), `jira jql validate` checks a query " +
		"string against Jira, and `jira jql reference` prints the field and operator " +
		"cheatsheet.\n\n" +
		"The built query is what `jira search jql` runs — compose it here, then pass it there."
	cmd.Example = `$ jira jql build --project ENG --status "In Progress"

# Validate a hand-written query
$ jira jql validate "project = ENG AND assignee = currentUser()"`
	build := &cobra.Command{
		Use:   "build",
		Short: "Build a JQL query from flags",
		Long: "Compose a JQL string from jira-cli's shared filter flags. Use it to preview " +
			"the query that `issue list` and related commands would run, or to build a " +
			"query for another tool.\n\n" +
			"This command is local and does not contact Jira. Board scope is applied after " +
			"the base query is built, and the returned URL is best-effort from the active " +
			"profile's base URL.",
		Example: `$ jira jql build --project PROJ --status "In Progress"

# Build a query for your own recently updated issues
$ jira jql build --assignee me --updated -7d --order-by updated

# Use a comma-separated issue-key filter
$ jira jql build --key PROJ-1,PROJ-2 --label backend

# Return the generated query and URL as JSON
$ jira jql build --project PROJ --status Done --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, precedence, err := boardscope.FromFlags(cmd)
			if err != nil {
				return err
			}
			query, err := builder.Build()
			if err != nil {
				return err
			}
			query = boardscope.ApplyClauseToJQL(query, scope)
			// Deep link built offline from the profile base URL, best-effort:
			// `jql build` never calls Jira and must still work without a fully
			// configured profile, so an unresolvable profile just yields no URL.
			url := ""
			if profile, perr := cmdutil.ProfileForCommand(cmd); perr == nil {
				url = browser.SearchURL(profile.BaseURL, query)
			}
			data := map[string]any{
				"jql":         query,
				"url":         url,
				"precedence":  precedence,
				"board_scope": boardscope.EnvelopeData(scope),
			}
			return cmdutil.WriteEnvelope(cmd, "jql.build", data)
		},
	}
	AddJQLBuilderFlags(build, &builder)
	boardscope.AddFlags(build)
	cmd.AddCommand(build)
	cmd.AddCommand(validateCommand())
	cmd.AddCommand(referenceCommand())
	return cmd
}

// referenceCommand lists the JQL metadata this instance exposes — every
// queryable field (including custom fields), function, and reserved word — via
// GET /jql/autocompletedata. Human output is one "value — displayName" line per
// field (the headline: discover queryable/custom fields); functions and
// reserved words ride along in the JSON envelope. Needs a configured profile.
func referenceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reference",
		Short: "List Jira JQL reference data",
		Long: "Fetch Jira's JQL autocomplete metadata for the active site. Use it to find " +
			"queryable field names, custom field IDs, functions, and reserved words before " +
			"writing a query.\n\n" +
			"The command calls Jira's `/jql/autocompletedata` endpoint and requires a " +
			"configured profile. Human output focuses on fields; JSON output includes " +
			"fields, functions, and reserved words.",
		Example: `$ jira jql reference

# Include functions and reserved words, not just human field rows
$ jira jql reference --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("validation: jql reference queries Jira and needs a configured profile")
			}
			var (
				ref  jira.JQLReference
				resp *jira.Response
			)
			if err := cmdutil.Spin(cmd, "jql.reference", func(ctx context.Context) error {
				var e error
				ref, resp, e = cmdutil.ServicesForClient(client).JQL().AutocompleteData(ctx)
				return e
			}); err != nil {
				return err
			}
			fields := make([]map[string]any, len(ref.Fields))
			for i, f := range ref.Fields {
				entry := map[string]any{"value": f.Value, "display_name": f.DisplayName}
				if f.CustomFieldID != "" {
					entry["custom_field_id"] = f.CustomFieldID
				}
				fields[i] = entry
			}
			functions := make([]map[string]any, len(ref.Functions))
			for i, fn := range ref.Functions {
				functions[i] = map[string]any{"value": fn.Value, "display_name": fn.DisplayName}
			}
			reserved := ref.ReservedWords
			if reserved == nil {
				reserved = []string{}
			}
			data := map[string]any{
				"fields":         fields,
				"functions":      functions,
				"reserved_words": reserved,
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "jql.reference", data, resp)
		},
	}
	return cmd
}

// validateCommand checks each JQL argument through Jira's parser via
// POST /jql/parse, reporting per-query errors and warnings. When the parse
// call succeeds it emits a successful envelope carrying data.queries[].valid —
// a JQL that fails to parse is a result, not a CLI input error, and is
// deliberately NOT mapped onto the local flag-validation exit code (that
// distinction is a separate design call). Consumers branch on
// data.queries[].valid rather than the exit code. Transport/API failures
// (auth, 429, 5xx) still surface as ordinary non-zero errors. Needs a
// configured profile.
func validateCommand() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "validate QUERY...",
		Short: "Validate JQL with Jira",
		Long: "Send one or more JQL strings to Jira's parser and report per-query errors and " +
			"warnings. Use it before saving a query or handing generated JQL to another " +
			"command.\n\n" +
			"A JQL parse failure is returned as data with `valid: false`; it is not treated " +
			"as a CLI input error. Transport, authentication, rate limit, and server " +
			"failures still return non-zero command errors.",
		Example: `$ jira jql validate "status = Done AND assignee = currentUser()"

# Use Jira's lenient warning mode
$ jira jql validate "project = PROJ" "created >= -7d" --mode warn

# Return per-query validity in JSON
$ jira jql validate "project = PROJ AND status = Done" --output=json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch mode {
			case "strict", "warn", "none":
			default:
				return fmt.Errorf("validation: --mode must be strict, warn, or none")
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("validation: validate queries Jira and needs a configured profile")
			}
			results, resp, err := cmdutil.ServicesForClient(client).JQL().Parse(cmd.Context(), args, mode)
			if err != nil {
				return err
			}
			out := make([]map[string]any, len(results))
			for i, r := range results {
				entry := map[string]any{"query": r.Query, "valid": len(r.Errors) == 0}
				if len(r.Errors) > 0 {
					entry["errors"] = r.Errors
				}
				if len(r.Warnings) > 0 {
					entry["warnings"] = r.Warnings
				}
				out[i] = entry
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "jql.validate", map[string]any{"queries": out}, resp)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &mode, "mode", "strict", "Validation strictness", clib.FlagExtra{
		Group:       "Validation",
		Placeholder: "MODE",
		Terse:       "validation mode",
		Enum:        []string{"strict", "warn", "none"},
		EnumTerse:   []string{"strictest", "lenient", "no validation"},
	})
	return cmd
}

// AddFilterFlags attaches the JQL filter flags shared by every command that
// builds a query — every field except assignee/reporter. `issue mine` pins
// assignee = currentUser() in its runner, so exposing --assignee would fight
// the pin and --reporter has no place on an assignee-scoped command; those two
// are the only flags AddJQLBuilderFlags adds on top. Because every other
// filter is registered here once, a new filter appears on both `issue list`
// and `issue mine` by construction. Sort and date flags compose via
// AddSortFlags / AddDateFilterFlags, which both commands also call.
func AddFilterFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Projects, "project", nil, "Restrict to Jira project key",
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheproject,comma"})
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Keys, "key", nil, "Restrict to issue key, comma list, or `PROJ-1..PROJ-5` range",
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=issuekey,comma"})
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Epics, "epic", nil, "Restrict to issues in epic keys",
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheepic,comma"})
	// Completion offers the plain status/priority names from the per-profile
	// cache — the common case, and the operand the negation form ("!Abandoned")
	// also takes. It deliberately does not cover the category-comparator grammar
	// ("<Done", ">=In Progress"), whose operand is a status *category*, not a
	// status name; a prefix like "<" simply matches no cached name and offers
	// nothing rather than a misleading one. The predictors emit names only, so
	// the short Terse (not the long usage) becomes each candidate's description.
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Statuses, "status", nil, "Restrict by status: name, category comparator (`<Done`, `>=In Progress`), or negation (`!Abandoned`)",
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Terse: "by status", Complete: "predictor=cachestatus,comma"})
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Priorities, "priority", nil, "Restrict by priority",
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Terse: "by priority", Complete: "predictor=cachepriority,comma"})
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.Labels, "label", nil, "Restrict by label",
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachelabel,comma"})
	cmdutil.AddStringSliceVar(cmd.Flags(), &builder.IssueTypes, "type", nil, "Restrict by issue type",
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Terse: "by type", Complete: "predictor=cacheissuetype,comma"})
}

// AddJQLBuilderFlags attaches the full JQL builder surface to cmd: the shared
// filter flags, plus the assignee/reporter filters that `issue mine` omits, and
// the sort and date flags.
func AddJQLBuilderFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	AddFilterFlags(cmd, builder)
	cmdutil.AddStringVar(cmd.Flags(), &builder.Assignee, "assignee", "", "Restrict by assignee; use `me` for the current user",
		clib.FlagExtra{
			Group:       "Filters",
			Placeholder: "USER",
			Terse:       "by assignee",
			Enum:        []string{"me", "none"},
			EnumTerse:   []string{"current user", "unassigned"},
		})
	cmdutil.AddStringVar(cmd.Flags(), &builder.Reporter, "reporter", "", "Restrict by reporter; use `me` for the current user",
		clib.FlagExtra{
			Group:       "Filters",
			Placeholder: "USER",
			Terse:       "by reporter",
			Enum:        []string{"me"},
			EnumTerse:   []string{"current user"},
		})
	AddSortFlags(cmd, builder)
	AddDateFilterFlags(cmd, builder)
}

// AddSortFlags attaches the --order-by/--desc sort flags to cmd. Shared by the
// full builder flag set and by `issue mine`, which carries its own reduced flag
// surface, so the sort defaults (field "updated", descending) are defined once
// and stay consistent across every command that builds a query.
func AddSortFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	cmdutil.AddStringVar(cmd.Flags(), &builder.OrderBy, "order-by", "updated", "Sort field",
		clib.FlagExtra{
			Group:       "Sort",
			Placeholder: "FIELD",
			Terse:       "sort field",
			Enum:        []string{"updated", "created", "priority", "status", "key", "summary"},
			EnumTerse:   []string{"last-updated time", "creation time", "priority level", "workflow status", "issue key", "title text"},
			EnumDefault: "updated",
		})
	cmdutil.AddBoolVar(cmd.Flags(), &builder.Descending, "desc", true, "Sort descending", clib.FlagExtra{Group: "Sort"})
}

// AddDateFilterFlags attaches the --updated/--created/--resolved timeframe
// flags to cmd. Shared by the full builder flag set and by `issue mine`, which
// carries its own reduced flag surface, so the date-flag grammar is defined
// once. Each value is a relative duration (-7d), an absolute date (2026-01-01),
// a comparator form (>=2026-01-01), or an A..B range — see jql.BuildOptions.
func AddDateFilterFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	// "Filters/Dates" renders the three date flags as a blank-line-separated
	// cluster at the foot of the Filters section, so the timeframe controls read
	// as a unit rather than interleaving with the project/status/label filters.
	cmdutil.AddStringVar(cmd.Flags(), &builder.Updated, "updated", "", "Filter by updated date: `-7d`, `2026-01-01`, `>=2026-01-01`, or `A..B` range",
		clib.FlagExtra{Group: "Filters/Dates", Placeholder: "DATE"})
	cmdutil.AddStringVar(cmd.Flags(), &builder.Created, "created", "", "Filter by created date (same grammar as `--updated`)",
		clib.FlagExtra{Group: "Filters/Dates", Placeholder: "DATE"})
	cmdutil.AddStringVar(cmd.Flags(), &builder.Resolved, "resolved", "", "Filter by resolved date (same grammar as `--updated`)",
		clib.FlagExtra{Group: "Filters/Dates", Placeholder: "DATE"})
}

// ReadCacheJSON loads a cache resource into v for the given profile, ignoring
// TTL so completion stays fast even when the cache is stale. Returns false
// silently on any error so completion never blocks the shell.
func ReadCacheJSON(profile, resource string, v any) bool {
	entry, ok, err := cache.ReadCachedOrEmpty(profile, resource)
	if err != nil || !ok {
		return false
	}
	return json.Unmarshal(entry.Data, v) == nil
}
