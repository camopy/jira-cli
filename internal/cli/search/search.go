package search

import (
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/browser"
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
	fields []string
	full   bool
	web    bool
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
			fields, detail, err := searchOutputFields(opts)
			if err != nil {
				return err
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				issues, resp, err := cmdutil.ServicesForClient(client).Search().JQL(cmd.Context(), &jira.SearchRequest{
					JQL:         args[0],
					Fields:      fields,
					ListOptions: jira.ListOptions{MaxResults: 50},
				})
				if err != nil {
					return err
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
