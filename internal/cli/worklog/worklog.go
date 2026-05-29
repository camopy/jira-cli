package worklog

import (
	"context"
	"encoding/json"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func worklogAddCommand() *cobra.Command {
	var timeSpent, commentMarkdown, started, jsonInput string
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "add KEY...",
		Short: "Add a worklog",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
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
			if len(keys) > 1 {
				return runWorklogAddMany(cmd, keys, parallelism, worklogAddInputs{
					TimeSpentSeconds: seconds,
					Started:          started,
					Comment:          comment,
					DryRun:           dryRun,
					Warnings:         pipeOut.Warnings,
				})
			}
			key := keys[0]
			if !dryRun {
				client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					worklog, resp, err := cmdutil.WorklogService(client).Add(cmd.Context(), key, &jira.WorklogAddRequest{TimeSpentSeconds: seconds, Started: started, Comment: comment})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "worklog.add", map[string]any{"issue": key, "worklog": worklog, "dry_run": false}, resp, pipeOut.Warnings)
				}
				return fmt.Errorf("jira base URL is required for worklog.add")
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "worklog.add", map[string]any{
				"issue": key,
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
	cmdutil.ExtendFlag(cmd.Flags(), "time-spent", clib.FlagExtra{Group: "Worklog", Placeholder: "DURATION"})
	cmdutil.ExtendFlag(cmd.Flags(), "started", clib.FlagExtra{Group: "Worklog", Placeholder: "TIME"})
	cmdutil.ExtendFlag(cmd.Flags(), "comment-markdown", clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	cmdutil.ExtendFileFlag(cmd.Flags(), "json-input", "Input", "FILE")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

type worklogAddInputs struct {
	TimeSpentSeconds int
	Started          string
	Comment          *adf.Document
	Warnings         []adf.Warning
	DryRun           bool
}

func runWorklogAddMany(cmd *cobra.Command, keys []string, parallelism int, in worklogAddInputs) error {
	if in.DryRun {
		results := make([]cmdutil.KeyResult[map[string]any], len(keys))
		for i, key := range keys {
			results[i] = cmdutil.KeyResult[map[string]any]{
				Key: key,
				Value: map[string]any{
					"issue": key,
					"worklog": map[string]any{
						"time_spent_seconds": in.TimeSpentSeconds,
						"started":            in.Started,
						"comment":            in.Comment,
					},
					"dry_run": true,
				},
			}
		}
		return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.add", results, cmdutil.KeyedDataWithWarnings(in.Warnings))
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for worklog.add")
	}
	service := cmdutil.WorklogService(client)
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		worklog, _, err := service.Add(ctx, key, &jira.WorklogAddRequest{
			TimeSpentSeconds: in.TimeSpentSeconds,
			Started:          in.Started,
			Comment:          in.Comment,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"issue": key, "worklog": worklog, "dry_run": false}, nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.add", results, cmdutil.KeyedDataWithWarnings(in.Warnings))
}

// NewCommand returns the `worklog` command group for managing issue worklogs.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("worklog", "Manage issue worklogs", "resources")
	cmd.AddCommand(worklogAddCommand())
	cmd.AddCommand(worklogListCommand())
	return cmd
}

func worklogListCommand() *cobra.Command {
	var parallelism int
	cmd := &cobra.Command{
		Use:   "list KEY...",
		Short: "List worklogs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				service := cmdutil.WorklogService(client)
				if len(keys) == 1 {
					worklogs, resp, err := service.List(cmd.Context(), keys[0], &jira.ListOptions{MaxResults: 50})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponse(cmd, "worklog.list", worklogListData(keys[0], worklogs), resp)
				}
				results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) ([]*jira.Worklog, error) {
					worklogs, _, err := service.List(ctx, key, &jira.ListOptions{MaxResults: 50})
					return worklogs, err
				})
				if err != nil {
					return err
				}
				return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.list", results, func(key string, worklogs []*jira.Worklog) any {
					return worklogListData(key, worklogs)
				})
			}
			if len(keys) == 1 {
				return cmdutil.WriteEnvelope(cmd, "worklog.list", worklogListData(keys[0], []any{}))
			}
			results := make([]cmdutil.KeyResult[any], 0, len(keys))
			for _, key := range keys {
				results = append(results, cmdutil.KeyResult[any]{Key: key, Value: []any{}})
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.list", results, func(key string, worklogs any) any {
				return worklogListData(key, worklogs)
			})
		},
	}
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func worklogListData(key string, worklogs any) map[string]any {
	return map[string]any{
		"issue":    key,
		"worklogs": worklogs,
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
