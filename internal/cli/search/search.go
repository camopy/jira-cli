package search

import (
	"context"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/jql"
	"github.com/spf13/cobra"
)

// NewCommand returns the `search` command group for running Jira searches.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("search", "Run Jira searches", "resources")
	cmd.AddCommand(searchJQLCommand())
	cmd.AddCommand(searchSavedCommand())
	return cmd
}

type searchOptions struct {
	fields    []string
	full      bool
	web       bool
	count     bool
	all       bool
	limit     int
	unbounded bool
	cursor    string
}

func searchJQLCommand() *cobra.Command {
	var opts searchOptions
	cmd := &cobra.Command{
		Use:   "jql QUERY",
		Short: "Run a JQL query",
		Long: "Run an inline JQL query against Jira and print matching issues. Use it when " +
			"you already have a query string from `jira jql build`, a saved filter, or a " +
			"Jira URL.\n\n" +
			"`--web` builds and opens the Jira search URL without running the query. " +
			"`--count` asks Jira for an approximate match count without fetching issues. " +
			"`--all` drains pages with default caps unless `--unbounded` is set.",
		Example: `$ jira search jql "status = Done AND assignee = currentUser()"

# Select only the fields you need
$ jira search jql "project = PROJ" --fields summary,status

# Ask Jira for an approximate count without fetching issues
$ jira search jql "project = PROJ" --count

# Restrict fields and keep the result parseable
$ jira search jql "project = PROJ AND status != Done" --fields key,summary,status --output=json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.web {
				return openSearchWeb(cmd, args[0])
			}
			if opts.count {
				return runSearchCount(cmd, args[0])
			}
			fields, detail, err := searchOutputFields(opts)
			if err != nil {
				return err
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				svc := cmdutil.ServicesForClient(client).Search()
				limit := opts.limit
				if limit <= 0 {
					limit = 50
				}
				req := &jira.SearchRequest{JQL: args[0], Fields: fields, ListOptions: jira.ListOptions{MaxResults: limit, NextPageToken: opts.cursor}} // pagination-exempt: opaque --cursor pass-through
				if opts.all {
					var (
						issues []*jira.Issue
						info   jira.DrainInfo
					)
					err = cmdutil.Spin(cmd, "search.jql", func(ctx context.Context) error {
						var spinErr error
						issues, info, spinErr = jira.DrainSearch(ctx, svc, req, jira.DrainOptions{Unbounded: opts.unbounded})
						return spinErr
					})
					if err != nil {
						return err
					}
					data := map[string]any{"source": "inline", "jql": args[0], "issues": cmdutil.IssueOutput(issues, detail)}
					// The drain knows its terminal state: the result set is
					// complete unless a bound truncated it. /search/jql has no
					// reliable total, so report the count we actually hold —
					// and the resume cursor when a bound cut the walk on a
					// page boundary.
					pagination := &cli.Pagination{
						MaxResults: len(issues),
						Total:      cli.KnownTotal(len(issues)),
						IsLast:     !info.Truncated,
						NextCursor: info.NextPageToken, // pagination-exempt: opaque resume token from the drain
					}
					return cmdutil.WriteEnvelopeWithPaginationAndRawWarnings(cmd, "search.jql", data, pagination, cmdutil.DrainTruncationWarnings(info))
				}
				var (
					found2 []*jira.Issue
					resp   *jira.Response
				)
				err = cmdutil.Spin(cmd, "search.jql", func(ctx context.Context) error {
					var spinErr error
					found2, resp, spinErr = svc.JQL(ctx, req)
					return spinErr
				})
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "search.jql", map[string]any{"source": "inline", "jql": args[0], "issues": cmdutil.IssueOutput(found2, detail)}, resp)
			}
			return cmdutil.WriteEnvelope(cmd, "search.jql", map[string]any{
				"source": "inline",
				"jql":    args[0],
				"issues": []any{},
			})
		},
	}
	addSearchOutputFlags(cmd, &opts)
	addSearchCountFlag(cmd, &opts)
	addSearchPaginationFlags(cmd, &opts)
	return cmd
}

func searchSavedCommand() *cobra.Command {
	var opts searchOptions
	cmd := &cobra.Command{
		Use:   "saved NAME",
		Short: "Run a saved JQL query",
		Long: "Load a named query from the configured queries file and run it against Jira. " +
			"Use it for team or personal searches that are too long to keep in shell " +
			"history.\n\n" +
			"The saved command uses the same output field selectors as inline search, but " +
			"does not implement `--count` or full pagination controls.",
		Example: `$ jira search saved my-open-bugs

# Run a saved query and select only the fields you need
$ jira search saved my-open-bugs --fields summary,status

# Keep saved-query results parseable
$ jira search saved my-open-bugs --output=json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields, detail, err := searchOutputFields(opts)
			if err != nil {
				return err
			}
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			queries, err := config.LoadQueries(cfg.QueriesPath)
			if err != nil {
				return err
			}
			query, ok := queries[args[0]]
			if !ok {
				return fmt.Errorf("saved query %q not found", args[0])
			}
			client, _, hasClient, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			issues := any([]any{})
			var resp *jira.Response
			if hasClient {
				var found []*jira.Issue
				err = cmdutil.Spin(cmd, "search.jql", func(ctx context.Context) error {
					var spinErr error
					found, resp, spinErr = cmdutil.ServicesForClient(client).Search().JQL(ctx, &jira.SearchRequest{
						JQL:         query.JQL,
						Fields:      fields,
						ListOptions: jira.ListOptions{MaxResults: 50},
					})
					return spinErr
				})
				if err != nil {
					return err
				}
				issues = cmdutil.IssueOutput(found, detail)
			}
			data := map[string]any{
				"source":      "saved",
				"key":         args[0],
				"name":        query.Name,
				"description": query.Description,
				"project":     query.Project,
				"jql":         query.JQL,
				"issues":      issues,
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "search.saved", data, resp)
		},
	}
	addSearchOutputFlags(cmd, &opts)
	return cmd
}

func addSearchOutputFlags(cmd *cobra.Command, opts *searchOptions) {
	fs := cmd.Flags()
	cmdutil.AddStringSliceVar(fs, &opts.fields, "fields", nil, "Issue fields to request from Jira, comma-separated [example: summary,status,assignee]", clib.FlagExtra{Group: "Output", Placeholder: "FIELD", Complete: "predictor=cachefield,comma"})
	cmdutil.AddBoolVar(fs, &opts.full, "full", false, "Request Jira's full issue payload (`*all` fields)", clib.FlagExtra{Group: "Output"})
	cmdutil.AddBoolVar(fs, &opts.web, "web", false, "Open the query in a browser instead of printing results", clib.FlagExtra{Group: "Output"})
	cmd.MarkFlagsMutuallyExclusive("fields", "full")
}

// addSearchPaginationFlags attaches --all/--limit/--unbounded. Like --count,
// they live only on `search jql`, not the shared output flags, so `search
// saved` doesn't publish flags its runner ignores.
func addSearchPaginationFlags(cmd *cobra.Command, opts *searchOptions) {
	fs := cmd.Flags()
	cmdutil.AddBoolVar(fs, &opts.all, "all", false, "Walk every page until `isLast` (bounded; use `--unbounded` to lift the caps)", clib.FlagExtra{Group: "Pagination"})
	cmdutil.AddIntVar(fs, &opts.limit, "limit", 50, "Page size requested from Jira", clib.FlagExtra{Group: "Pagination", Placeholder: "N"})
	cmdutil.AddBoolVar(fs, &opts.unbounded, "unbounded", false, "With `--all`, lift the default 100-page / 10 000-issue caps", clib.FlagExtra{Group: "Pagination"})
	cmdutil.AddStringVar(fs, &opts.cursor, "cursor", "", "Resume from a `nextCursor` returned by a previous page", clib.FlagExtra{Group: "Pagination", Placeholder: "TOKEN"})
	// --count fetches nothing and --web opens a browser, so the page controls
	// are meaningless alongside either. --cursor composes with --limit (page
	// size of the resumed page) and --all (resume the drain from the cursor).
	cmd.MarkFlagsMutuallyExclusive("count", "all")
	cmd.MarkFlagsMutuallyExclusive("count", "limit")
	cmd.MarkFlagsMutuallyExclusive("count", "cursor")
	cmd.MarkFlagsMutuallyExclusive("web", "all")
	cmd.MarkFlagsMutuallyExclusive("web", "limit")
	cmd.MarkFlagsMutuallyExclusive("web", "cursor")
}

// Drain truncation warnings are shared via cmdutil.DrainTruncationWarnings —
// `issue list --all` emits the identical contract.

// addSearchCountFlag attaches --count. It lives only on `search jql`, not on the
// shared output flags, because `search saved` does not implement count — adding
// it there would publish a flag the saved runner silently ignores. Must be
// called after addSearchOutputFlags so the flags it conflicts with exist.
func addSearchCountFlag(cmd *cobra.Command, opts *searchOptions) {
	cmdutil.AddBoolVar(cmd.Flags(), &opts.count, "count", false, "Return only the approximate match count, without fetching issues", clib.FlagExtra{Group: "Output"})
	// --count fetches no issues, so the field/full selectors and the browser
	// opener are all meaningless alongside it.
	cmd.MarkFlagsMutuallyExclusive("count", "fields")
	cmd.MarkFlagsMutuallyExclusive("count", "full")
	cmd.MarkFlagsMutuallyExclusive("count", "web")
}

// runSearchCount fetches Jira's approximate match count for jqlStr and emits it
// without retrieving any issues. Unlike `--web` and `--as-jql`-style previews,
// the count comes from Jira, so a configured profile is required.
func runSearchCount(cmd *cobra.Command, jqlStr string) error {
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validation: --count queries Jira for the estimate and needs a configured profile")
	}
	var (
		count int
		resp  *jira.Response
	)
	err = cmdutil.Spin(cmd, "search.count", func(ctx context.Context) error {
		var spinErr error
		count, resp, spinErr = cmdutil.ServicesForClient(client).Search().ApproximateCount(ctx, jqlStr)
		return spinErr
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteEnvelopeWithResponse(cmd, "search.count", map[string]any{
		"source": "inline",
		"jql":    jqlStr,
		"count":  count,
	}, resp)
}

// openSearchWeb builds the JQL search URL from the active profile and opens it
// in a browser when interactive, reporting the URL in the envelope either way.
// It needs no Jira call — only the configured base URL.
func openSearchWeb(cmd *cobra.Command, jqlQuery string) error {
	profile, err := cmdutil.ProfileForCommand(cmd)
	if err != nil {
		return err
	}
	u := browser.SearchURL(profile.BaseURL, jqlQuery)
	if u == "" {
		return fmt.Errorf("validation: opening a query in the browser requires a configured base URL")
	}
	return cmdutil.WriteWebEnvelope(cmd, "search.jql", u, map[string]any{"source": "inline", "jql": jqlQuery})
}

func searchOutputFields(opts searchOptions) ([]string, bool, error) {
	fields := jql.CompactStrings(opts.fields)
	if opts.full && len(fields) > 0 {
		return nil, false, fmt.Errorf("validation: --fields and --full are mutually exclusive")
	}
	if opts.full {
		return []string{"*all"}, true, nil
	}
	if len(fields) > 0 {
		return fields, true, nil
	}
	return jira.DefaultIssueListFields(), false, nil
}
