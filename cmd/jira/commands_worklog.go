package main

import (
	"encoding/json"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func worklogAddCommand() *cobra.Command {
	var timeSpent, commentMarkdown, started, jsonInput string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add KEY",
		Short: "Add a worklog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// commentADF carries a canonical ADF `comment` document
			// supplied via --json-input. It is mutually exclusive with
			// the Markdown comment form.
			var commentADF *adf.Document
			if jsonInput != "" {
				var input struct {
					TimeSpent       string          `json:"time_spent"`
					TimeSpentLegacy string          `json:"timeSpent"`
					Started         string          `json:"started"`
					CommentMarkdown string          `json:"comment_markdown"`
					Comment         json.RawMessage `json:"comment"`
				}
				if err := cmdutil.ReadJSONFile(jsonInput, &input); err != nil {
					return err
				}
				if input.TimeSpent == "" {
					input.TimeSpent = input.TimeSpentLegacy
				}
				if input.TimeSpent != "" && !cmd.Flags().Changed("time-spent") {
					timeSpent = input.TimeSpent
				}
				if input.Started != "" && !cmd.Flags().Changed("started") {
					started = input.Started
				}
				if input.CommentMarkdown != "" && !cmd.Flags().Changed("comment-markdown") {
					commentMarkdown = input.CommentMarkdown
				}
				// `comment` is the canonical ADF document shape — the
				// same shape `issue comment --json-input` accepts. It is
				// parsed and validated through the pipeline below.
				if len(input.Comment) > 0 && string(input.Comment) != "null" {
					if input.CommentMarkdown != "" {
						return fmt.Errorf("validation: worklog input has both 'comment' (ADF) and 'comment_markdown'; provide exactly one")
					}
					parsed, _, perr := adf.Parse(input.Comment)
					if perr != nil {
						return fmt.Errorf("worklog --json-input comment: %w", perr)
					}
					commentADF = &parsed
				}
			}
			seconds, err := jira.ParseDuration(timeSpent, workdaySecondsForCommand(cmd))
			if err != nil {
				return err
			}
			var comment *adf.Document
			var commentMarkdownWarnings []adf.Warning
			switch {
			case commentADF != nil:
				comment = commentADF
			case commentMarkdown != "":
				doc, convWarnings, err := adf.FromMarkdownLossy(commentMarkdown)
				if err != nil {
					return err
				}
				comment = &doc
				commentMarkdownWarnings = convWarnings
			}
			// Thread worklog comment ADF through the pipeline.
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:             cmdutil.ADFModeFor(cmd, true),
				ADFDoc:           comment,
				MarkdownWarnings: commentMarkdownWarnings,
				DryRun:           dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Submit the validated SubmitADF (post-compatibility),
			// not the pre-pipeline comment doc.
			comment = pipeOut.SubmitADF
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					worklog, resp, err := worklogService(client).Add(cmd.Context(), args[0], &jira.WorklogAddRequest{TimeSpentSeconds: seconds, Started: started, Comment: comment})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "worklog.add", map[string]any{"issue": args[0], "worklog": worklog, "dry_run": false}, resp, pipeOut.Warnings)
				}
				return fmt.Errorf("jira base URL is required for worklog.add")
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "worklog.add", map[string]any{
				"issue": args[0],
				"worklog": map[string]any{
					"time_spent_seconds": seconds,
					"started":            started,
					"comment":            comment,
				},
				"dry_run": dryRun,
			}, pipeOut.Warnings)
		},
	}
	cmd.Flags().StringVar(&timeSpent, "time-spent", "", "Human-readable time spent")
	cmd.Flags().StringVar(&started, "started", "", "Worklog start timestamp")
	cmd.Flags().StringVar(&commentMarkdown, "comment-markdown", "", "Worklog comment as Markdown")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read worklog payload from JSON file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	extendFlag(cmd.Flags(), "time-spent", clib.FlagExtra{Group: "Worklog", Placeholder: "DURATION"})
	extendFlag(cmd.Flags(), "started", clib.FlagExtra{Group: "Worklog", Placeholder: "TIME"})
	extendFlag(cmd.Flags(), "comment-markdown", clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	extendFileFlag(cmd.Flags(), "json-input", "Input", "FILE")
	extendDryRunFlag(cmd.Flags())
	return cmd
}

func worklogCommand() *cobra.Command {
	cmd := groupCommand("worklog", "Manage issue worklogs", "resources")
	cmd.AddCommand(worklogAddCommand())
	cmd.AddCommand(worklogListCommand())
	return cmd
}

func worklogListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list KEY",
		Short: "List worklogs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				worklogs, resp, err := worklogService(client).List(cmd.Context(), args[0], &jira.ListOptions{MaxResults: 50})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "worklog.list", map[string]any{"issue": args[0], "worklogs": worklogs}, resp)
			}
			return cmdutil.WriteEnvelope(cmd, "worklog.list", map[string]any{
				"issue":    args[0],
				"worklogs": []any{},
			})
		},
	}
}

func workdaySecondsForCommand(cmd *cobra.Command) int {
	cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
	if err != nil {
		return config.DefaultWorkdaySeconds
	}
	profile := cmdutil.ActiveProfile(cmd, cfg)
	if profile.WorkdaySeconds <= 0 {
		return config.DefaultWorkdaySeconds
	}
	return profile.WorkdaySeconds
}
