package epic

import (
	"context"
	"fmt"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/spf13/cobra"
)

var epicIssueKeyArg = map[string]string{"clib": "dynamic-args='issuekey'"}

// NewCommand returns the `epic` command group: list, board, add, and remove.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("epic", "Work with Jira epics", "resources")
	cmd.AddCommand(epicListCommand())
	cmd.AddCommand(epicBoardCommand())
	cmd.AddCommand(epicAddCommand())
	cmd.AddCommand(epicRemoveCommand())
	return cmd
}

func epicListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List epics",
		Long: "List visible Jira epics using the standard epic JQL for the active profile. " +
			"Use it when you need epic keys for issue assignment or reporting.\n\n" +
			"If no Jira profile is configured, the command still returns the JQL and an " +
			"empty result set so agents can see the intended query shape.",
		Example: `$ jira epic list

# Return epic keys in a parseable shape
$ jira epic list --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				var epics []*jira.Issue
				var resp *jira.Response
				err = cmdutil.Spin(cmd, "epic.list", func(ctx context.Context) error {
					var spinErr error
					epics, resp, spinErr = cmdutil.ServicesForClient(client).Epic().List(ctx, &jira.ListOptions{MaxResults: 50})
					return spinErr
				})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.list", map[string]any{"jql": jql.EpicListJQL, "epics": epics, "detail": false}, resp)
			}
			return cmdutil.WriteEnvelope(cmd, "epic.list", map[string]any{
				"jql":    jql.EpicListJQL,
				"epics":  []any{},
				"detail": false,
			})
		},
	}
}

func epicBoardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Show epic status counts",
		Long: "Build a compact epic board report by listing epics and counting each epic's " +
			"child issues by status. Use it when you need a terminal summary rather than " +
			"opening Jira in a browser.\n\n" +
			"The command performs one child-issue lookup per epic and is capped by the " +
			"epic list limit. If no Jira profile is configured, it returns empty rows and " +
			"zero totals.",
		Example: `$ jira epic board

# Include per-epic counts for scripts
$ jira epic board --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.WriteEnvelope(cmd, "epic.board", map[string]any{
					"epics":  []any{},
					"totals": emptyEpicCounts(),
				})
			}
			service := cmdutil.ServicesForClient(client).Epic()
			var epics []*jira.Issue
			err = cmdutil.Spin(cmd, "epic.list", func(ctx context.Context) error {
				var spinErr error
				epics, _, spinErr = service.List(ctx, &jira.ListOptions{MaxResults: 50})
				return spinErr
			})
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(epics))
			totals := emptyEpicCounts()
			for _, epic := range epics {
				key := ""
				summary := ""
				status := ""
				if epic != nil && epic.Key != nil {
					key = *epic.Key
				}
				if epic != nil && epic.Fields != nil {
					if epic.Fields.Summary != nil {
						summary = *epic.Fields.Summary
					}
					if epic.Fields.Status != nil && epic.Fields.Status.Name != nil {
						status = *epic.Fields.Status.Name
					}
				}
				var children []*jira.Issue
				err = cmdutil.Spin(cmd, "epic.board", func(ctx context.Context) error {
					var spinErr error
					children, _, spinErr = service.IssuesInEpic(ctx, key)
					return spinErr
				})
				if err != nil {
					return err
				}
				counts := jira.StatusCounts(children)
				for status, n := range counts {
					totals[status] += n
				}
				rows = append(rows, map[string]any{
					"key":     key,
					"summary": summary,
					"status":  status,
					"counts":  counts,
				})
			}
			return cmdutil.WriteEnvelope(cmd, "epic.board", map[string]any{
				"epics":  rows,
				"totals": totals,
			})
		},
	}
}

func emptyEpicCounts() map[string]int {
	return map[string]int{"To Do": 0, "In Progress": 0, "Done": 0}
}

func epicAddCommand() *cobra.Command {
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "add ISSUE_KEY... EPIC_KEY",
		Short: "Add an issue to an epic",
		Long: "Assign one or more issues to an epic. Use it when triaging work into a " +
			"known epic key from `jira epic list` or Jira.\n\n" +
			"`--dry-run` previews the issue-to-epic assignment locally and does not " +
			"contact Jira. Multiple issue keys are processed with bounded parallelism and " +
			"return per-key results.",
		Args: cobra.MinimumNArgs(2),
		Example: `$ jira epic add PROJ-123 PROJ-100

# Apply one epic assignment to several issues
$ jira epic add PROJ-123 PROJ-124 PROJ-125 PROJ-100

# Preview the assignment without contacting Jira
$ jira epic add PROJ-123 PROJ-100 --dry-run`,
		Annotations: epicIssueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			epicKey := args[len(args)-1]
			keys, err := issuekey.ParseExpressions(args[:len(args)-1], issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			if len(keys) > 1 {
				return runEpicAddMany(cmd, keys, epicKey, parallelism, dryRun)
			}
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					var resp *jira.Response
					err = cmdutil.Spin(cmd, "epic.add", func(ctx context.Context) error {
						var spinErr error
						resp, spinErr = cmdutil.ServicesForClient(client).Epic().AddIssue(ctx, epicKey, keys[0])
						return spinErr
					})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.add", epicAddData(keys[0], epicKey, false), resp)
				}
				return fmt.Errorf("jira base URL is required for epic.add")
			}
			return cmdutil.WriteEnvelope(cmd, "epic.add", epicAddData(keys[0], epicKey, true))
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview mutation without submitting")
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func runEpicAddMany(cmd *cobra.Command, keys []string, epicKey string, parallelism int, dryRun bool) error {
	if dryRun {
		results := make([]cmdutil.KeyResult[map[string]any], len(keys))
		for i, key := range keys {
			results[i] = cmdutil.KeyResult[map[string]any]{Key: key, Value: epicAddData(key, epicKey, true)}
		}
		return cmdutil.WriteKeyedResultsEnvelope(cmd, "epic.add", results, func(_ string, data map[string]any) any { return data })
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for epic.add")
	}
	service := cmdutil.ServicesForClient(client).Epic()
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		if _, err := service.AddIssue(ctx, epicKey, key); err != nil {
			return nil, err
		}
		return epicAddData(key, epicKey, false), nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "epic.add", results, func(_ string, data map[string]any) any { return data })
}

func epicAddData(issueKey, epicKey string, dryRun bool) map[string]any {
	return map[string]any{
		"issue":   issueKey,
		"epic":    epicKey,
		"dry_run": dryRun,
		"added":   !dryRun,
	}
}

func epicRemoveCommand() *cobra.Command {
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "remove ISSUE_KEY...",
		Short: "Remove an issue from its epic",
		Long: "Remove one or more issues from their current epic. Use it when work was " +
			"assigned to the wrong epic or should stand alone.\n\n" +
			"`--dry-run` previews the removals locally and does not contact Jira. Multiple " +
			"issue keys are processed with bounded parallelism and return per-key results.",
		Args: cobra.MinimumNArgs(1),
		Example: `$ jira epic remove PROJ-123

# Apply the removal to several issues
$ jira epic remove PROJ-123 PROJ-124 PROJ-125

# Preview the removal without contacting Jira
$ jira epic remove PROJ-123 --dry-run`,
		Annotations: epicIssueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			if len(keys) > 1 {
				return runEpicRemoveMany(cmd, keys, parallelism, dryRun)
			}
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					var resp *jira.Response
					err = cmdutil.Spin(cmd, "epic.remove", func(ctx context.Context) error {
						var spinErr error
						resp, spinErr = cmdutil.ServicesForClient(client).Epic().RemoveIssue(ctx, keys[0])
						return spinErr
					})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.remove", epicRemoveData(keys[0], false), resp)
				}
				return fmt.Errorf("jira base URL is required for epic.remove")
			}
			return cmdutil.WriteEnvelope(cmd, "epic.remove", epicRemoveData(keys[0], true))
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview mutation without submitting")
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func runEpicRemoveMany(cmd *cobra.Command, keys []string, parallelism int, dryRun bool) error {
	if dryRun {
		results := make([]cmdutil.KeyResult[map[string]any], len(keys))
		for i, key := range keys {
			results[i] = cmdutil.KeyResult[map[string]any]{Key: key, Value: epicRemoveData(key, true)}
		}
		return cmdutil.WriteKeyedResultsEnvelope(cmd, "epic.remove", results, func(_ string, data map[string]any) any { return data })
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for epic.remove")
	}
	service := cmdutil.ServicesForClient(client).Epic()
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		if _, err := service.RemoveIssue(ctx, key); err != nil {
			return nil, err
		}
		return epicRemoveData(key, false), nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "epic.remove", results, func(_ string, data map[string]any) any { return data })
}

func epicRemoveData(issueKey string, dryRun bool) map[string]any {
	return map[string]any{
		"issue":   issueKey,
		"dry_run": dryRun,
		"removed": !dryRun,
	}
}
