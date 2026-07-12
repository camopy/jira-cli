package issue

import (
	"context"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
)

func issueRankCommand() *cobra.Command {
	var before, after string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rank KEY... (--before ANCHOR | --after ANCHOR)",
		Short: "Reorder backlog issues relative to an anchor issue",
		Long: "Move one or more issues so they sit immediately before or after an anchor " +
			"issue in backlog order (Jira's LexoRank: a sortable rank string per issue, " +
			"so reordering never renumbers the rest) — the headless equivalent of dragging rows " +
			"in the web UI's backlog. The issues keep the order you pass them in.\n\n" +
			"Exactly one anchor is required: `--before` places the issues above it, " +
			"`--after` below it. Key lists and ranges expand like every multi-key command, " +
			"and more than 50 keys are chunked transparently with the order preserved " +
			"end-to-end. Verify the result with `jira issue list --jql \"project = X ORDER " +
			"BY Rank ASC\"`.\n\n" +
			"Ranking is Jira Software board functionality: a project with no board (no " +
			"rank field) rejects it, as does an anchor you cannot rank against. " +
			"`--dry-run` previews the submission order locally and never contacts Jira.",
		Example: `$ jira issue rank PROJ-7 PROJ-9 --before PROJ-3

# Move a range to the bottom of another issue
$ jira issue rank PROJ-20:24 --after PROJ-50

# Preview the order without contacting Jira
$ jira issue rank PROJ-7 PROJ-9 --before PROJ-3 --dry-run --output=json`,
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			anchor, position := before, "before"
			if after != "" {
				anchor, position = after, "after"
			}
			if _, err := issuekey.ParseExpressions([]string{anchor}, issuekey.Options{MaxExpansion: 1}); err != nil {
				return err
			}
			cmdutil.RecordIssueKeys(cmd, append(append([]string{}, keys...), anchor)...)
			data := map[string]any{
				"anchor":   anchor,
				"position": position,
				"order":    keys,
				"chunks":   (len(keys) + jira.RankChunkLimit - 1) / jira.RankChunkLimit,
			}
			if dryRun {
				data["dry_run"] = true
				return cmdutil.WriteEnvelope(cmd, "issue.rank", data)
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				data["ranked"] = false
				return cmdutil.WriteEnvelope(cmd, "issue.rank", data)
			}
			service := cmdutil.ServicesForClient(client).Rank()
			if err := cmdutil.Spin(cmd, "issue.rank", func(ctx context.Context) error {
				return rankInChunks(ctx, service, keys, position, anchor)
			}); err != nil {
				return err
			}
			data["dry_run"] = false
			return cmdutil.WriteEnvelope(cmd, "issue.rank", data)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &before, "before", "",
		"Anchor issue the ranked issues are placed immediately above", clib.FlagExtra{
			Group:       "Rank",
			Placeholder: "ANCHOR",
			Terse:       "rank above anchor",
			Complete:    "predictor=issuekey",
		})
	cmdutil.AddStringVar(cmd.Flags(), &after, "after", "",
		"Anchor issue the ranked issues are placed immediately below", clib.FlagExtra{
			Group:       "Rank",
			Placeholder: "ANCHOR",
			Terse:       "rank below anchor",
			Complete:    "predictor=issuekey",
		})
	cmd.MarkFlagsMutuallyExclusive("before", "after")
	cmd.MarkFlagsOneRequired("before", "after")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview the submission order without contacting Jira")
	return cmd
}

// rankInChunks submits keys in RankChunkLimit-sized chunks, sequentially:
// the first chunk uses the caller's anchor and direction, and every later
// chunk ranks after the last key of the chunk before it, so the requested
// order survives however many requests it takes. Chunks after a failure do
// not run — their anchor (the previous chunk's tail) never landed.
func rankInChunks(ctx context.Context, service jira.RankService, keys []string, position, anchor string) error {
	for start := 0; start < len(keys); start += jira.RankChunkLimit {
		end := min(start+jira.RankChunkLimit, len(keys))
		chunk := keys[start:end]
		before, after := "", ""
		switch {
		case start > 0:
			after = keys[start-1]
		case position == "before":
			before = anchor
		default:
			after = anchor
		}
		if _, err := service.Rank(ctx, chunk, before, after); err != nil {
			if start > 0 {
				// Earlier chunks committed; the caller needs to know the
				// partial state to resume rather than re-rank everything.
				return fmt.Errorf("the first %d issues already ranked before the failure: %w", start, err)
			}
			return err
		}
	}
	return nil
}
