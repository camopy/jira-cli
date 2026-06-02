package search

import (
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
}

func searchJQLCommand() *cobra.Command {
	var opts searchOptions
	cmd := &cobra.Command{
		Use:   "jql QUERY",
		Short: "Run a JQL query",
		Args:  cobra.ExactArgs(1),
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
				req := &jira.SearchRequest{JQL: args[0], Fields: fields, ListOptions: jira.ListOptions{MaxResults: limit}}
				if opts.all {
					issues, info, derr := jira.DrainSearch(cmd.Context(), svc, req, jira.DrainOptions{Unbounded: opts.unbounded})
					if derr != nil {
						return derr
					}
					data := map[string]any{"source": "inline", "jql": args[0], "issues": cmdutil.IssueOutput(issues, detail)}
					// The drain knows its terminal state: the result set is
					// complete unless a bound truncated it. /search/jql has no
					// reliable total, so report the count we actually hold.
					pagination := &cli.Pagination{
						MaxResults: len(issues),
						Total:      len(issues),
						IsLast:     !info.Truncated,
					}
					return cmdutil.WriteEnvelopeWithPaginationAndRawWarnings(cmd, "search.jql", data, pagination, searchTruncationWarnings(info))
				}
				issues, resp, jerr := svc.JQL(cmd.Context(), req)
				if jerr != nil {
					return jerr
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "search.jql", map[string]any{"source": "inline", "jql": args[0], "issues": cmdutil.IssueOutput(issues, detail)}, resp)
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
		Args:  cobra.ExactArgs(1),
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
				found, response, err := cmdutil.ServicesForClient(client).Search().JQL(cmd.Context(), &jira.SearchRequest{
					JQL:         query.JQL,
					Fields:      fields,
					ListOptions: jira.ListOptions{MaxResults: 50},
				})
				if err != nil {
					return err
				}
				issues = cmdutil.IssueOutput(found, detail)
				resp = response
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
	cmd.Flags().StringSliceVar(&opts.fields, "fields", nil, "Issue fields to request from Jira (comma-separated)")
	cmd.Flags().BoolVar(&opts.full, "full", false, `Request Jira's full issue payload with fields ["*all"]`)
	cmd.Flags().BoolVar(&opts.web, "web", false, "Open the query in a browser instead of printing results")
	cmd.MarkFlagsMutuallyExclusive("fields", "full")
	clib.Extend(cmd.Flags().Lookup("fields"), clib.FlagExtra{Group: "Output", Placeholder: "FIELD", Complete: "predictor=cachefield,comma"})
	clib.Extend(cmd.Flags().Lookup("full"), clib.FlagExtra{Group: "Output"})
	clib.Extend(cmd.Flags().Lookup("web"), clib.FlagExtra{Group: "Output"})
}

// addSearchPaginationFlags attaches --all/--limit/--unbounded. Like --count,
// they live only on `search jql`, not the shared output flags, so `search
// saved` doesn't publish flags its runner ignores.
func addSearchPaginationFlags(cmd *cobra.Command, opts *searchOptions) {
	cmd.Flags().BoolVar(&opts.all, "all", false, "Walk every page until isLast (bounded; use --unbounded to lift the caps)")
	cmd.Flags().IntVar(&opts.limit, "limit", 50, "Page size requested from Jira")
	cmd.Flags().BoolVar(&opts.unbounded, "unbounded", false, "With --all, lift the default 100-page / 10 000-issue caps")
	// --count fetches nothing and --web opens a browser, so the page controls
	// are meaningless alongside either.
	cmd.MarkFlagsMutuallyExclusive("count", "all")
	cmd.MarkFlagsMutuallyExclusive("count", "limit")
	cmd.MarkFlagsMutuallyExclusive("web", "all")
	cmd.MarkFlagsMutuallyExclusive("web", "limit")
	cmdutil.ExtendPaginationFlags(cmd.Flags())
	clib.Extend(cmd.Flags().Lookup("unbounded"), clib.FlagExtra{Group: "Pagination"})
}

// searchTruncationWarnings maps a bounded-drain truncation onto the envelope's
// warnings[], with a re-run-with---unbounded remediation. nil when the drain
// reached isLast.
func searchTruncationWarnings(info jira.DrainInfo) []map[string]any {
	if !info.Truncated {
		return nil
	}
	limit := 100
	if info.TruncatedReason == "max_results" {
		limit = 10_000
	}
	return []map[string]any{{
		"type":          "search-truncated",
		"resource":      "issues",
		"reason":        info.TruncatedReason,
		"limit":         limit,
		"pages_fetched": info.PagesFetched,
		"message":       "search truncated by " + info.TruncatedReason + "; re-run with --unbounded to fetch every issue",
		"remediation":   "Re-run with --unbounded if you need every issue.",
	}}
}

// addSearchCountFlag attaches --count. It lives only on `search jql`, not on the
// shared output flags, because `search saved` does not implement count — adding
// it there would publish a flag the saved runner silently ignores. Must be
// called after addSearchOutputFlags so the flags it conflicts with exist.
func addSearchCountFlag(cmd *cobra.Command, opts *searchOptions) {
	cmd.Flags().BoolVar(&opts.count, "count", false, "Return only the approximate match count, without fetching issues")
	// --count fetches no issues, so the field/full selectors and the browser
	// opener are all meaningless alongside it.
	cmd.MarkFlagsMutuallyExclusive("count", "fields")
	cmd.MarkFlagsMutuallyExclusive("count", "full")
	cmd.MarkFlagsMutuallyExclusive("count", "web")
	clib.Extend(cmd.Flags().Lookup("count"), clib.FlagExtra{Group: "Output"})
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
	count, resp, err := cmdutil.ServicesForClient(client).Search().ApproximateCount(cmd.Context(), jqlStr)
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
