package main

import (
	"fmt"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

func epicCommand() *cobra.Command {
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				epics, resp, err := epicService(client).List(cmd.Context(), &jira.ListOptions{MaxResults: 50})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.list", map[string]any{"jql": "issuetype = Epic", "epics": epics, "detail": false}, resp)
			}
			return cmdutil.WriteEnvelope(cmd, "epic.list", map[string]any{
				"jql":    "issuetype = Epic",
				"epics":  []any{},
				"detail": false,
			})
		},
	}
}

func epicBoardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Open the epic board",
		Args:  cobra.NoArgs,
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
			service := epicService(client)
			epics, _, err := service.List(cmd.Context(), &jira.ListOptions{MaxResults: 50})
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
				children, _, err := service.IssuesInEpic(cmd.Context(), key)
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
	cmd := &cobra.Command{
		Use:   "add ISSUE_KEY EPIC_KEY",
		Short: "Add an issue to an epic",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					resp, err := epicService(client).AddIssue(cmd.Context(), args[1], args[0])
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.add", map[string]any{"issue": args[0], "epic": args[1], "dry_run": false, "added": true}, resp)
				}
				return fmt.Errorf("jira base URL is required for epic.add")
			}
			return cmdutil.WriteEnvelope(cmd, "epic.add", map[string]any{
				"issue":   args[0],
				"epic":    args[1],
				"dry_run": dryRun,
				"added":   !dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}

func epicRemoveCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remove ISSUE_KEY",
		Short: "Remove an issue from its epic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					resp, err := epicService(client).RemoveIssue(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponse(cmd, "epic.remove", map[string]any{"issue": args[0], "dry_run": false, "removed": true}, resp)
				}
				return fmt.Errorf("jira base URL is required for epic.remove")
			}
			return cmdutil.WriteEnvelope(cmd, "epic.remove", map[string]any{
				"issue":   args[0],
				"dry_run": dryRun,
				"removed": !dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}
