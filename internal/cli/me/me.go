package me

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
)

// NewCommand returns the `me` command: a short alias for the identity
// portion of `jira auth whoami`. It fetches /myself for the active profile
// and prints a compact identity envelope. Distinct from `auth whoami`, which
// also offers `--save` and is intentionally namespaced under `auth`.
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the active profile's Jira identity",
		Long: "Fetch `/myself` for the active profile and print the resolved Jira account " +
			"identity. Use it for a quick read-only check of the profile selected by " +
			"`--profile` or config.",
		Example: `$ jira me

# Check a non-active profile and keep output parseable
$ jira --profile prod me --output=json`,
		GroupID: "configuration",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for me")
			}
			var user *jira.CurrentUser
			err = cmdutil.Spin(cmd, "me", func(ctx context.Context) error {
				var spinErr error
				user, _, spinErr = cmdutil.ServicesForClient(client).User().Myself(ctx)
				return spinErr
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "me", map[string]any{
				"profile":       profile.Name,
				"account_id":    user.AccountID,
				"display_name":  user.DisplayName,
				"email_address": user.EmailAddress,
				"time_zone":     user.TimeZone,
			})
		},
	}
}
