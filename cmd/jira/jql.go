package main

import (
	"encoding/json"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/spf13/cobra"
)

func jqlCommand() *cobra.Command {
	var builder jql.BuildOptions
	cmd := cmdutil.GroupCommand("jql", "Build JQL queries", "resources")
	build := &cobra.Command{
		Use:   "build",
		Short: "Build a JQL query from flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, precedence, err := boardScopeFromFlags(cmd)
			if err != nil {
				return err
			}
			query, err := builder.Build()
			if err != nil {
				return err
			}
			query = applyBoardClauseToJQL(query, scope)
			data := map[string]any{
				"jql":         query,
				"precedence":  precedence,
				"board_scope": boardScopeEnvelopeData(scope),
			}
			return cmdutil.WriteEnvelope(cmd, "jql.build", data)
		},
	}
	addJQLBuilderFlags(build, &builder)
	addBoardScopeFlags(build)
	cmd.AddCommand(build)
	return cmd
}

func addJQLBuilderFlags(cmd *cobra.Command, builder *jql.BuildOptions) {
	cmd.Flags().StringSliceVar(&builder.Projects, "project", nil, "Restrict to Jira project key")
	cmd.Flags().StringSliceVar(&builder.Epics, "epic", nil, "Restrict to issues in epic keys")
	cmd.Flags().StringVar(&builder.Assignee, "assignee", "", `Restrict by assignee; use "me" for currentUser()`)
	cmd.Flags().StringVar(&builder.Reporter, "reporter", "", `Restrict by reporter; use "me" for currentUser()`)
	cmd.Flags().StringSliceVar(&builder.Statuses, "status", nil, "Restrict by status name")
	cmd.Flags().StringSliceVar(&builder.Priorities, "priority", nil, "Restrict by priority")
	cmd.Flags().StringSliceVar(&builder.Labels, "label", nil, "Restrict by label")
	cmd.Flags().StringSliceVar(&builder.IssueTypes, "type", nil, "Restrict by issue type")
	cmd.Flags().StringVar(&builder.OrderBy, "order-by", "updated", "Sort field")
	cmd.Flags().BoolVar(&builder.Descending, "desc", true, "Sort descending")
	clib.Extend(cmd.Flags().Lookup("project"), clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheproject,comma"})
	clib.Extend(cmd.Flags().Lookup("epic"), clib.FlagExtra{Group: "Filters", Placeholder: "KEY", Complete: "predictor=cacheepic,comma"})
	clib.Extend(cmd.Flags().Lookup("label"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cachelabel,comma"})
	clib.Extend(cmd.Flags().Lookup("type"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cacheissuetype,comma"})
	clib.Extend(cmd.Flags().Lookup("assignee"), clib.FlagExtra{Group: "Filters", Placeholder: "USER", Enum: []string{"me", "none"}})
	clib.Extend(cmd.Flags().Lookup("reporter"), clib.FlagExtra{Group: "Filters", Placeholder: "USER", Enum: []string{"me"}})
	clib.Extend(cmd.Flags().Lookup("status"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME"})
	clib.Extend(cmd.Flags().Lookup("priority"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME"})
	clib.Extend(cmd.Flags().Lookup("order-by"), clib.FlagExtra{Group: "Sort", Placeholder: "FIELD", Enum: []string{"updated", "created", "priority", "status", "key", "summary"}, EnumDefault: "updated"})
	clib.Extend(cmd.Flags().Lookup("desc"), clib.FlagExtra{Group: "Sort"})
}

// readCacheJSON loads a cache resource into v for the given profile, ignoring
// TTL so completion stays fast even when the cache is stale. Returns false
// silently on any error so completion never blocks the shell.
func readCacheJSON(profile, resource string, v any) bool {
	entry, ok, _, err := cache.Read(profile, resource, 24*time.Hour*365)
	if err != nil || !ok {
		return false
	}
	return json.Unmarshal(entry.Data, v) == nil
}
