package cmdutil

import (
	"slices"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/clib/complete"
	"github.com/gechr/clib/help"
	"github.com/gechr/clib/theme"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewHelpRenderer builds the themed clib help renderer used by every command
// in jira-cli. The JIRA env-prefix is set so that JIRA_NO_COLOR and related
// variables are honored. config.DefaultTheme resolves the JIRA_THEME override
// (or the dark built-in), which clib v0.5 no longer does for us after dropping
// the old theme.Default().
func NewHelpRenderer() *help.Renderer {
	theme.SetEnvPrefix("JIRA")
	th := config.DefaultTheme().With(
		theme.WithEnumStyle(theme.EnumStyleHighlightBoth),
		theme.WithHelpRepeatEllipsisEnabled(true),
	)
	return help.NewRenderer(th)
}

// StandardHelpSections returns the standard clib help sections for cmd,
// with subcommand listing made optional and the Output group rendered last.
func StandardHelpSections(cmd *cobra.Command) []help.Section {
	sections := clib.SectionsWithOptions(clib.WithSubcommandOptional())(cmd)
	if cmd == nil || !cmd.Runnable() || !cmd.HasSubCommands() || helpSectionsContainFlags(sections) {
		return moveOutputSectionLast(sections)
	}

	flagSections := runnableParentFlagSections(cmd)
	if len(flagSections) == 0 {
		return moveOutputSectionLast(sections)
	}
	markUsageWithOptions(sections)
	return moveOutputSectionLast(append(sections, flagSections...))
}

// moveOutputSectionLast reorders the flag sections so the "Output" group is the
// last domain group. clib renders groups in first-seen registration order,
// which otherwise places Output wherever its first flag happened to be
// declared (e.g. before "Filters"). Output is re-inserted just before the
// trailing generic "Options"/"Global Options" block so it reads last among the
// command's own groups while the conventional -h/--help line stays at the very
// bottom.
func moveOutputSectionLast(sections []help.Section) []help.Section {
	idx := -1
	for i := range sections {
		if sections[i].Title == "Output" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return sections
	}
	target := sections[idx]
	out := slices.Delete(slices.Clone(sections), idx, idx+1)
	insertAt := len(out)
	for i := range out {
		if out[i].Title == "Options" || out[i].Title == "Global Options" {
			insertAt = i
			break
		}
	}
	return slices.Insert(out, insertAt, target)
}

func helpSectionsContainFlags(sections []help.Section) bool {
	for _, section := range sections {
		for _, content := range section.Content {
			if _, ok := content.(help.FlagGroup); ok {
				return true
			}
		}
	}
	return false
}

func markUsageWithOptions(sections []help.Section) {
	for i := range sections {
		for j := range sections[i].Content {
			usage, ok := sections[i].Content[j].(help.Usage)
			if !ok {
				continue
			}
			usage.ShowOptions = true
			sections[i].Content[j] = usage
		}
	}
}

func runnableParentFlagSections(cmd *cobra.Command) []help.Section {
	metaByName := make(map[string]complete.FlagMeta)
	for _, meta := range clib.FlagMeta(cmd) {
		metaByName[meta.Name] = meta
	}

	var classified []help.ClassifiedFlag
	seen := make(map[string]struct{})
	addFlags := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			if flag.Hidden {
				return
			}
			if _, ok := seen[flag.Name]; ok {
				return
			}
			seen[flag.Name] = struct{}{}
			meta := metaByName[flag.Name]
			classified = append(classified, help.ClassifiedFlag{
				Flag:  helpFlagFromPFlag(flag, meta),
				Group: meta.Group,
			})
		})
	}
	addFlags(cmd.LocalNonPersistentFlags())
	addFlags(cmd.PersistentFlags())

	if len(classified) == 0 {
		return nil
	}
	return help.BuildFlagSections(classified, help.WithKeepGroupOrder())
}

func helpFlagFromPFlag(flag *pflag.Flag, meta complete.FlagMeta) help.Flag {
	out := help.Flag{
		Short:         flag.Shorthand,
		Long:          flag.Name,
		Desc:          flag.Usage,
		Enum:          meta.Enum,
		EnumDefault:   meta.EnumDefault,
		EnumHighlight: meta.EnumHighlight,
		NoIndent:      meta.NoIndent,
	}
	if meta.HideLong {
		out.Long = ""
	}
	if meta.HideShort {
		out.Short = ""
	}
	if meta.HasArg {
		out.Placeholder = meta.Placeholder
		if out.Placeholder == "" {
			out.Placeholder = flag.Name
		}
		out.Repeatable = meta.IsSlice
	}
	return out
}
