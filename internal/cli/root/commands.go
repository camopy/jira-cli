package root

import (
	"github.com/matcra587/jira-cli/internal/cli/adfcmd"
	"github.com/matcra587/jira-cli/internal/cli/agent"
	"github.com/matcra587/jira-cli/internal/cli/alias"
	"github.com/matcra587/jira-cli/internal/cli/auth"
	"github.com/matcra587/jira-cli/internal/cli/boards"
	"github.com/matcra587/jira-cli/internal/cli/cache"
	"github.com/matcra587/jira-cli/internal/cli/config"
	"github.com/matcra587/jira-cli/internal/cli/epic"
	"github.com/matcra587/jira-cli/internal/cli/issue"
	"github.com/matcra587/jira-cli/internal/cli/jql"
	"github.com/matcra587/jira-cli/internal/cli/me"
	"github.com/matcra587/jira-cli/internal/cli/releasenotes"
	"github.com/matcra587/jira-cli/internal/cli/search"
	"github.com/matcra587/jira-cli/internal/cli/tui"
	"github.com/matcra587/jira-cli/internal/cli/update"
	"github.com/matcra587/jira-cli/internal/cli/user"
	"github.com/matcra587/jira-cli/internal/cli/version"
	"github.com/matcra587/jira-cli/internal/cli/worklog"
	"github.com/spf13/cobra"
)

func registerCommands(root *cobra.Command) {
	root.AddCommand(
		tui.NewCommand(),
		agent.NewCommand(),
		adfcmd.NewCommand(),
		cache.NewCommand(),
		me.NewCommand(),
		version.NewCommand(),
		update.NewCommand(),
		auth.NewCommand(),
		issue.NewCommand(),
		issue.NewOpenCommand(),
		boards.NewCommand(),
		epic.NewCommand(),
		jql.NewCommand(),
		alias.NewCommand(),
		search.NewCommand(),
		releasenotes.NewCommand(),
		user.NewCommand(),
		worklog.NewCommand(),
		config.NewCommand(),
	)
}
