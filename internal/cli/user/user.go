package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
)

// NewCommand returns the `user` group. Its one subcommand, `search`,
// resolves a display name or email to Jira account identities — the missing
// step between "mention someone" and the accountId an ADF mention node
// requires. Deliberately read-only: authoring stays on the issue and
// comment commands.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("user", "Look up Jira users", "resources")
	cmd.AddCommand(searchCommand())
	return cmd
}

func searchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search QUERY",
		Short: "Search users by display name or email",
		Long: "Search Jira users by display name or email and return their account " +
			"identities. The `account_id` in each match is the value an ADF `mention` " +
			"node's `attrs.id` requires — this command is the deterministic path from a " +
			"person's name or email to a mention.\n\n" +
			"An email query resolves to a single match wherever the address is unique " +
			"on the instance. Name queries may return several candidates; every match " +
			"is returned so the caller can disambiguate on `email_address` or " +
			"`display_name`. Inactive and deleted accounts are excluded.",
		Example: `# Resolve an email to an accountId
$ jira user search "dev@example.com" --output=json

# Find candidates by display name
$ jira user search "Sam" --output=json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for user.search")
			}
			var users []*jira.User
			err = cmdutil.Spin(cmd, "user.search", func(ctx context.Context) error {
				var spinErr error
				users, _, spinErr = cmdutil.ServicesForClient(client).User().Search(ctx, args[0])
				return spinErr
			})
			if err != nil {
				return err
			}
			matches := make([]map[string]any, 0, len(users))
			for _, u := range users {
				if u == nil || !boolValue(u.Active) || boolValue(u.Deleted) {
					continue
				}
				matches = append(matches, map[string]any{
					"account_id":    stringValue(u.AccountID),
					"display_name":  stringValue(u.DisplayName),
					"email_address": stringValue(u.EmailAddress),
				})
			}
			return cmdutil.WriteEnvelope(cmd, "user.search", map[string]any{
				"query": args[0],
				"users": matches,
				"count": len(matches),
			})
		},
	}
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func boolValue(b *bool) bool {
	return b != nil && *b
}
