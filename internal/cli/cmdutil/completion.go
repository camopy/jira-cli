package cmdutil

import (
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// CompleteProfileNames completes the names of all profiles in the config. It
// is shared by every command that accepts a profile argument (e.g. config and
// auth). On any config-load error it returns no completions rather than an
// error, so the shell degrades gracefully.
func CompleteProfileNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		out = append(out, p.Name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
