package worklog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	xslices "github.com/gechr/x/slices"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/envelope"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

// issueKeyArg wires dynamic-args='issuekey' so the worklog commands that take
// an issue KEY positionally participate in key completion, matching the issue
// subcommands. The predictor is a no-op until an issue-key cache lands, but the
// annotation keeps the surface consistent.
var issueKeyArg = map[string]string{"clib": "dynamic-args='issuekey'"}

func worklogAddCommand() *cobra.Command {
	var timeSpent, commentMarkdown, commentMarkdownFile, started, jsonInput string
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:         "add KEY...",
		Annotations: issueKeyArg,
		Short:       "Add a worklog",
		Long: "Add a worklog to one or more issues. Use it when recording time spent from the " +
			"terminal, with optional Markdown or ADF comments.\n\n" +
			"Time is parsed with the active profile's workday length for day-based values. " +
			"Comments pass through the validate-and-encode ADF pipeline before submission.\n\n" +
			"`--dry-run` runs local parsing and ADF validation but does not contact Jira. " +
			"`--json-input` can supply time, started, and a canonical ADF comment for " +
			"headless workflows.",
		Example: `$ jira worklog add PROJ-123 --time-spent 2h30m

# Include a Markdown comment in the worklog
$ jira worklog add PROJ-123 --time-spent 1h --markdown "Pairing session"

# Preview parsing and ADF validation without contacting Jira
$ jira worklog add PROJ-123 --time-spent 45m --dry-run

# Use a JSON payload for headless input
$ jira worklog add PROJ-123 --json-input worklog.json --dry-run --output=json`,
		Args: cobra.MinimumNArgs(1),
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
				// The markdown flags are mutually exclusive with --json-input,
				// so a payload comment_markdown can never race a flag value.
				if input.CommentMarkdown != "" {
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
			// Normalize --started before the dry-run split so a value Jira
			// would reject fails local validation (exit 3) instead of the
			// submit, and the preview echoes the exact wire timestamp.
			if started != "" {
				normalized, err := jira.ParseStarted(started, time.Now(), time.Local)
				if err != nil {
					return fmt.Errorf("validation: --started: %w", err)
				}
				started = normalized
			}
			var comment *adf.Document
			resolvedMarkdown, mdErr := cmdutil.ResolveMarkdownInput(commentMarkdown, commentMarkdownFile)
			if mdErr != nil {
				return mdErr
			}
			commentMarkdown = resolvedMarkdown
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
					var worklog *jira.Worklog
					var resp *jira.Response
					err = cmdutil.Spin(cmd, "worklog.add", func(ctx context.Context) error {
						var spinErr error
						worklog, resp, spinErr = cmdutil.ServicesForClient(client).Worklog().Add(ctx, key, &jira.WorklogAddRequest{TimeSpentSeconds: seconds, Started: started, Comment: comment})
						return spinErr
					})
					if err != nil {
						return err
					}
					return cmdutil.WriteEnvelopeWithResponseAndWarnings(cmd, "worklog.add", envelope.WorklogAddOutput{Issue: cmdutil.IssueRef{Key: key}, Worklog: worklog, DryRun: false}, resp, pipeOut.Warnings)
				}
				return fmt.Errorf("jira base URL is required for worklog.add")
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "worklog.add", envelope.WorklogAddOutput{
				Issue: cmdutil.IssueRef{Key: key},
				Worklog: &envelope.WorklogDraft{
					TimeSpentSeconds: seconds,
					Started:          started,
					Comment:          comment,
				},
				DryRun: dryRun,
			}, pipeOut.Warnings)
		},
	}
	fs := cmd.Flags()
	cmdutil.AddStringVar(fs, &timeSpent, "time-spent", "", "Human-readable time spent [example: 2h30m]", clib.FlagExtra{Group: "Worklog", Placeholder: "DURATION"})
	cmdutil.AddStringVar(fs, &started, "started", "", "Worklog start time: ISO-8601 (offset optional, local time assumed) or relative (`now`, `yesterday`, `2h ago`); omit to let Jira stamp now [example: 2026-06-26T10:00]", clib.FlagExtra{Group: "Worklog", Placeholder: "TIME"})
	cmdutil.AddFileFlag(fs, &jsonInput, "json-input", "", "Read worklog payload from JSON file (canonical for agents)", "Input", "FILE")
	cmdutil.AddDryRunFlag(fs, &dryRun, "Preview mutation without submitting")
	cmdutil.AddMarkdownFlag(cmd, &commentMarkdown, &commentMarkdownFile, "Worklog comment as Markdown", "comment-markdown")
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

// worklogAddResult folds the command-wide ADF warnings into a per-key
// worklog.add result. The single-key envelope carries these warnings at the
// envelope's warnings[]; on the batch path each resource result carries its
// own copy, since there is no single persisted resource to hang them on.
type worklogAddResult struct {
	envelope.WorklogAddOutput
	Warnings []adf.Warning `json:"warnings,omitempty"`
}

// worklogAddKeyedData returns the per-key data mapper for the batch envelope:
// the bare output when there are no warnings, or the output plus warnings when
// there are.
func worklogAddKeyedData(warnings []adf.Warning) func(string, envelope.WorklogAddOutput) any {
	return func(_ string, out envelope.WorklogAddOutput) any {
		if len(warnings) == 0 {
			return out
		}
		return worklogAddResult{WorklogAddOutput: out, Warnings: warnings}
	}
}

func runWorklogAddMany(cmd *cobra.Command, keys []string, parallelism int, in worklogAddInputs) error {
	if in.DryRun {
		results := xslices.Map(keys, func(key string) cmdutil.KeyResult[envelope.WorklogAddOutput] {
			return cmdutil.KeyResult[envelope.WorklogAddOutput]{
				Key: key,
				Value: envelope.WorklogAddOutput{
					Issue: cmdutil.IssueRef{Key: key},
					Worklog: &envelope.WorklogDraft{
						TimeSpentSeconds: in.TimeSpentSeconds,
						Started:          in.Started,
						Comment:          in.Comment,
					},
					DryRun: true,
				},
			}
		})
		return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.add", results, worklogAddKeyedData(in.Warnings))
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for worklog.add")
	}
	service := cmdutil.ServicesForClient(client).Worklog()
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (envelope.WorklogAddOutput, error) {
		worklog, _, err := service.Add(ctx, key, &jira.WorklogAddRequest{
			TimeSpentSeconds: in.TimeSpentSeconds,
			Started:          in.Started,
			Comment:          in.Comment,
		})
		if err != nil {
			return envelope.WorklogAddOutput{}, err
		}
		return envelope.WorklogAddOutput{Issue: cmdutil.IssueRef{Key: key}, Worklog: worklog, DryRun: false}, nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.add", results, worklogAddKeyedData(in.Warnings))
}

// NewCommand returns the `worklog` command group for managing issue worklogs.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("worklog", "Manage issue worklogs", "resources")
	cmd.Long = "Log and review time against issues. `jira worklog add` records time with a " +
		"duration and optional comment; `jira worklog list` shows the entries on an issue.\n\n" +
		"Durations use Jira workday semantics — `workday_seconds` on the profile sets the day " +
		"length, so `1d` is a working day, not 24 hours."
	cmd.Example = `$ jira worklog add PROJ-123 --time-spent 2h30m

# Review logged time on an issue
$ jira worklog list PROJ-123`
	cmd.AddCommand(worklogAddCommand())
	cmd.AddCommand(worklogListCommand())
	return cmd
}

func worklogListCommand() *cobra.Command {
	var parallelism int
	cmd := &cobra.Command{
		Use:         "list KEY...",
		Annotations: issueKeyArg,
		Short:       "List worklogs",
		Long: "List worklogs for one or more issues. Use it to inspect time entries before " +
			"adding more work or auditing recent activity.\n\n" +
			"Each issue fetch is capped at the command's page size. Multiple issue keys are " +
			"queried with bounded parallelism and return per-key results.",
		Example: `$ jira worklog list PROJ-123

# Fetch several issues with bounded parallelism
$ jira worklog list PROJ-1 PROJ-2 PROJ-3

# Keep worklog output parseable
$ jira worklog list PROJ-123 --output=json`,
		Args: cobra.MinimumNArgs(1),
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
				service := cmdutil.ServicesForClient(client).Worklog()
				if len(keys) == 1 {
					var worklogs []*jira.Worklog
					var resp *jira.Response
					err = cmdutil.Spin(cmd, "worklog.list", func(ctx context.Context) error {
						var spinErr error
						worklogs, resp, spinErr = service.List(ctx, keys[0], &jira.ListOptions{MaxResults: 50})
						return spinErr
					})
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
				return cmdutil.WriteEnvelope(cmd, "worklog.list", worklogListData(keys[0], []*jira.Worklog{}))
			}
			results := make([]cmdutil.KeyResult[[]*jira.Worklog], 0, len(keys))
			for _, key := range keys {
				results = append(results, cmdutil.KeyResult[[]*jira.Worklog]{Key: key, Value: []*jira.Worklog{}})
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "worklog.list", results, func(key string, worklogs []*jira.Worklog) any {
				return worklogListData(key, worklogs)
			})
		},
	}
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func worklogListData(key string, worklogs []*jira.Worklog) envelope.WorklogListOutput {
	return envelope.WorklogListOutput{
		Issue:    cmdutil.IssueRef{Key: key},
		Worklogs: worklogs,
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
