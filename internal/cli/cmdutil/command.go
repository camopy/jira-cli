package cmdutil

import (
	"github.com/spf13/cobra"
)

// GroupCommand returns a *cobra.Command suitable for use as a named
// subcommand group (no Run/RunE — only subcommands are registered on it).
func GroupCommand(use, short, group string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
	}
}
