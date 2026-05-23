package cmdutil

import (
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/clib/help"
	"github.com/gechr/clib/theme"
	"github.com/spf13/cobra"
)

// NewHelpRenderer builds the themed clib help renderer used by every command
// in jira-cli. The JIRA env-prefix is set so that JIRA_NO_COLOR and related
// variables are honored.
func NewHelpRenderer() *help.Renderer {
	theme.SetEnvPrefix("JIRA")
	th := theme.Default().With(
		theme.WithEnumStyle(theme.EnumStyleHighlightBoth),
		theme.WithHelpRepeatEllipsisEnabled(true),
	)
	return help.NewRenderer(th)
}

// StandardHelpSections returns the standard clib help sections for cmd,
// with subcommand listing made optional.
func StandardHelpSections(cmd *cobra.Command) []help.Section {
	return clib.SectionsWithOptions(clib.WithSubcommandOptional())(cmd)
}
