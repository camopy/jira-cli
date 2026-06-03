package jql

import (
	"encoding/json"
	"fmt"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/boardscope"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/spf13/cobra"
)

// NewCommand returns the `jql` command group for building JQL queries.
func NewCommand() *cobra.Command {
	var builder jql.BuildOptions
	cmd := cmdutil.GroupCommand("jql", "Build JQL queries", "resources")
	build := &cobra.Command{
		Use:   "build",
		Short: "Build a JQL query from flags",
		Args:  cobra.NoArgs,
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
		Short: "List the JQL fields, functions, and reserved words this instance exposes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("validation: jql reference queries Jira and needs a configured profile")
			}
			ref, resp, err := cmdutil.ServicesForClient(client).JQL().AutocompleteData(cmd.Context())
			if err != nil {
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
		Short: "Validate JQL through Jira's parser (per-query errors and warnings)",
		Args:  cobra.MinimumNArgs(1),
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
	cmd.Flags().StringVar(&mode, "mode", "strict", "Validation strictness: strict, warn, or none")
	clib.Extend(cmd.Flags().Lookup("mode"), clib.FlagExtra{
		Group:       "Validation",
		Placeholder: "MODE",
		Terse:       "validation mode",
		Enum:        []string{"strict", "warn", "none"},
		EnumTerse:   []string{"strictest", "warn, don't fail", "no validation"},
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
	cmd.Flags().StringSliceVar(&builder.Projects, "project", nil, "Restrict to Jira project key")
	cmd.Flags().StringSliceVar(&builder.Keys, "key", nil, "Restrict to issue key, comma list, or range")
	cmd.Flags().StringSliceVar(&builder.Epics, "epic", nil, "Restrict to issues in epic keys")
	cmd.Flags().StringSliceVar(&builder.Statuses, "status", nil, `Restrict by status name, category comparator ("<Done", ">=In Progress"), or negation ("!Abandoned")`)
	cmd.Flags().StringSliceVar(&builder.Priorities, "priority", nil, "Restrict by priority")
	cmd.Flags().StringSliceVar(&builder.Labels, "label", nil, "Restrict by label")
	cmd.Flags().StringSliceVar(&builder.IssueTypes, "type", nil, "Restrict by issue type")
	clib.Extend(
		cmd.Flags().Lookup("project"),
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheproject,comma"},
	)
	clib.Extend(
		cmd.Flags().Lookup("key"),
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=issuekey,comma"},
	)
	clib.Extend(
		cmd.Flags().Lookup("epic"),
		clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheepic,comma"},
	)
	clib.Extend(
		cmd.Flags().Lookup("label"),
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachelabel,comma"},
	)
	clib.Extend(
		cmd.Flags().Lookup("type"),
		clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cacheissuetype,comma"},
	)
	// Completion offers the plain status/priority names from the per-profile
	// cache — the common case, and the operand the negation form ("!Abandoned")
	// also takes. It deliberately does not cover the category-comparator grammar
	// ("<Done", ">=In Progress"), whose operand is a status *category*, not a
	// status name; a prefix like "<" simply matches no cached name and offers
	// nothing rather than a misleading one.
	clib.Extend(cmd.Flags().Lookup("status"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachestatus,comma"})
	clib.Extend(cmd.Flags().Lookup("priority"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachepriority,comma"})
}

// AddJQLBuilderFlags attaches the full JQL builder surface to cmd: the shared
// filter flags, plus the assignee/reporter filters that `issue mine` omits, and
// the sort and date flags.
func AddJQLBuilderFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	AddFilterFlags(cmd, builder)
	cmd.Flags().StringVar(&builder.Assignee, "assignee", "", `Restrict by assignee; use "me" for currentUser()`)
	cmd.Flags().StringVar(&builder.Reporter, "reporter", "", `Restrict by reporter; use "me" for currentUser()`)
	clib.Extend(
		cmd.Flags().Lookup("assignee"),
		clib.FlagExtra{
			Group:       "Filters",
			Placeholder: "USER",
			Terse:       "by assignee",
			Enum:        []string{"me", "none"},
			EnumTerse:   []string{"current user", "unassigned"},
		},
	)
	clib.Extend(
		cmd.Flags().Lookup("reporter"),
		clib.FlagExtra{
			Group:       "Filters",
			Placeholder: "USER",
			Terse:       "by reporter",
			Enum:        []string{"me"},
			EnumTerse:   []string{"current user"},
		},
	)
	AddSortFlags(cmd, builder)
	AddDateFilterFlags(cmd, builder)
}

// AddSortFlags attaches the --order-by/--desc sort flags to cmd. Shared by the
// full builder flag set and by `issue mine`, which carries its own reduced flag
// surface, so the sort defaults (field "updated", descending) are defined once
// and stay consistent across every command that builds a query.
func AddSortFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	cmd.Flags().StringVar(&builder.OrderBy, "order-by", "updated", "Sort field")
	cmd.Flags().BoolVar(&builder.Descending, "desc", true, "Sort descending")
	clib.Extend(
		cmd.Flags().Lookup("order-by"),
		clib.FlagExtra{
			Group:       "Sort",
			Placeholder: "FIELD",
			Terse:       "sort field",
			Enum:        []string{"updated", "created", "priority", "status", "key", "summary"},
			EnumTerse:   []string{"last-updated time", "creation time", "priority level", "workflow status", "issue key", "title text"},
			EnumDefault: "updated",
		},
	)
	clib.Extend(cmd.Flags().Lookup("desc"), clib.FlagExtra{Group: "Sort"})
}

// AddDateFilterFlags attaches the --updated/--created/--resolved timeframe
// flags to cmd. Shared by the full builder flag set and by `issue mine`, which
// carries its own reduced flag surface, so the date-flag grammar is defined
// once. Each value is a relative duration (-7d), an absolute date (2026-01-01),
// a comparator form (>=2026-01-01), or an A..B range — see jql.BuildOptions.
func AddDateFilterFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	cmd.Flags().StringVar(&builder.Updated, "updated", "", "Filter by updated date: -7d, 2026-01-01, >=2026-01-01, or A..B range")
	cmd.Flags().StringVar(&builder.Created, "created", "", "Filter by created date (same value grammar as --updated)")
	cmd.Flags().StringVar(&builder.Resolved, "resolved", "", "Filter by resolved date (same value grammar as --updated)")
	for _, name := range []string{"updated", "created", "resolved"} {
		clib.Extend(cmd.Flags().Lookup(name), clib.FlagExtra{Group: "Filters", Placeholder: "DATE"})
	}
}

// ReadCacheJSON loads a cache resource into v for the given profile, ignoring
// TTL so completion stays fast even when the cache is stale. Returns false
// silently on any error so completion never blocks the shell.
func ReadCacheJSON(profile, resource string, v any) bool {
	entry, ok, _, err := cache.Read(profile, resource, 24*time.Hour*365)
	if err != nil || !ok {
		return false
	}
	return json.Unmarshal(entry.Data, v) == nil
}
