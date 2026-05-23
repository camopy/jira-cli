package main

import (
	"os"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/version"
	"github.com/spf13/cobra"
)

func registerCommands(root *cobra.Command) {
	root.AddCommand(
		tuiCommand(),
		agentCommand(),
		cacheCommand(),
		meCommand(),
		versionCommand(),
		authCommand(),
		issueCommand(),
		boardsCommand(),
		epicCommand(),
		jqlCommand(),
		aliasCommand(),
		searchCommand(),
		worklogCommand(),
		configCommand(),
	)
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		GroupID: "agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdutil.WriteEnvelope(cmd, "version", map[string]any{
				"version":    version.Version,
				"commit":     version.Commit,
				"branch":     version.Branch,
				"build_time": version.BuildTime,
				"build_by":   version.BuildBy,
				"summary":    version.String(),
			})
		},
	}
}

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Short:   "Launch the persistent dashboard",
		GroupID: "dashboard",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cli.RequireTTY(os.Stdout)
			if err != nil {
				return err
			}
			_, err = tuiRun(cmd)
			return err
		},
	}
}
