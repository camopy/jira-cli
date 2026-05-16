package main

import (
	"errors"
	"fmt"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/pkg/jira"
	"github.com/spf13/cobra"
)

// issueKeyArg is the standard annotation map every new issue-key
// positional carries — wires future issuekey predictor without per-call
// boilerplate.
var issueKeyArg = map[string]string{"clib": "dynamic-args='issuekey'"}

// WatcherCommands is the public wiring slice the root command-tree
// dispatcher in commands.go consumes — keeps the registration surface
// explicit so each new sub-command lands in one place. Exposing it
// avoids "unused symbol" lint flags while incremental delivery is in
// flight.
var WatcherCommands = []func() *cobra.Command{
	issueWatcherCommand,
	issueWatchCommand,
	issueUnwatchCommand,
}

// issueWatcherCommand groups `jira issue watchers list/add/remove`.
// The watch / unwatch shortcuts are registered separately at the
// `issue` group level via WatcherCommands.
func issueWatcherCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watchers",
		Short: "Manage issue watchers",
	}
	cmd.AddCommand(watcherListCommand())
	cmd.AddCommand(watcherAddCommand())
	cmd.AddCommand(watcherRemoveCommand())
	return cmd
}

// issueWatchCommand wires the `jira issue watch KEY` shortcut — equivalent
// to `watchers add --user me`. Same envelope shape so consumers don't
// branch by command name.
func issueWatchCommand() *cobra.Command {
	var dryRun, noReadback bool
	cmd := &cobra.Command{
		Use:         "watch KEY",
		Short:       "Start watching an issue (alias for watchers add --user me)",
		Args:        cobra.ExactArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatcherAdd(cmd, watcherMutationArgs{
				Key: args[0], UserIdent: "me", DryRun: dryRun, NoReadback: noReadback,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without calling Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	return cmd
}

// issueUnwatchCommand wires `jira issue unwatch KEY` — equivalent to
// `watchers remove --user me`.
func issueUnwatchCommand() *cobra.Command {
	var dryRun, noReadback bool
	cmd := &cobra.Command{
		Use:         "unwatch KEY",
		Short:       "Stop watching an issue (alias for watchers remove --user me)",
		Args:        cobra.ExactArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatcherRemove(cmd, watcherMutationArgs{
				Key: args[0], UserIdent: "me", DryRun: dryRun, NoReadback: noReadback,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without calling Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	return cmd
}

// watcherMutationArgs bundles the inputs to runWatcherAdd / runWatcherRemove
// so the helpers stay at ≤2 visible parameters (cmd + args) — easier to grow
// later (e.g. --visibility on watcher add) without ballooning the signatures.
type watcherMutationArgs struct {
	Key        string
	UserIdent  string
	DryRun     bool
	NoReadback bool
}

// ----- list -----------------------------------------------------------------

func watcherListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "list KEY",
		Short:         "List watchers on an issue",
		Args:          cobra.ExactArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.watchers.list")
			}
			watchers, resp, err := jira.NewWatcherService(client).List(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			data := map[string]any{
				"watchers":    watcherListData(watchers.Watchers),
				"is_watching": watchers.IsWatching,
				"watch_count": watchers.WatchCount,
			}
			return writeEnvelopeWithResponse(cmd, "issue.watchers.list", data, resp)
		},
	}
	return cmd
}

// ----- add ------------------------------------------------------------------

func watcherAddCommand() *cobra.Command {
	var user string
	var dryRun, noReadback bool
	cmd := &cobra.Command{
		Use:           "add KEY",
		Short:         "Add a watcher to an issue",
		Args:          cobra.ExactArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if user == "" {
				return fmt.Errorf("validation: --user is required (me / accountId:<id> / email)")
			}
			return runWatcherAdd(cmd, watcherMutationArgs{
				Key: args[0], UserIdent: user, DryRun: dryRun, NoReadback: noReadback,
			})
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "User identifier (me / accountId:<id> / email)")
	clib.Extend(cmd.Flags().Lookup("user"), clib.FlagExtra{Placeholder: "IDENTIFIER"})
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without calling Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	return cmd
}

// ----- remove ---------------------------------------------------------------

func watcherRemoveCommand() *cobra.Command {
	var user string
	var dryRun, noReadback bool
	cmd := &cobra.Command{
		Use:           "remove KEY",
		Short:         "Remove a watcher from an issue",
		Args:          cobra.ExactArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if user == "" {
				return fmt.Errorf("validation: --user is required (me / accountId:<id> / email)")
			}
			return runWatcherRemove(cmd, watcherMutationArgs{
				Key: args[0], UserIdent: user, DryRun: dryRun, NoReadback: noReadback,
			})
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "User identifier (me / accountId:<id> / email)")
	clib.Extend(cmd.Flags().Lookup("user"), clib.FlagExtra{Placeholder: "IDENTIFIER"})
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without calling Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	return cmd
}

// ----- shared add/remove drivers -------------------------------------------

func runWatcherAdd(cmd *cobra.Command, args watcherMutationArgs) error {
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.watchers.add")
	}
	user := jira.NewUserService(client)

	if args.DryRun {
		// Dry-run still resolves the user when possible — gives the
		// operator confidence the supplied --user actually maps. If the
		// resolver hits ambiguity we surface that diagnostic so they
		// can fix it before re-running without --dry-run.
		accountID, rerr := user.ResolveUser(cmd.Context(), args.UserIdent)
		if rerr != nil {
			return handleResolveErr(cmd, "issue.watchers.add", rerr)
		}
		return writeEnvelope(cmd, "issue.watchers.add", map[string]any{
			"key":                 args.Key,
			"account_id_resolved": accountID,
			"dry_run":             true,
		})
	}

	// Resolve the user first — bailing on a not-found / ambiguous query
	// before the pre-state readback avoids a wasted /watchers GET on the
	// unhappy path.
	accountID, err := user.ResolveUser(cmd.Context(), args.UserIdent)
	if err != nil {
		return handleResolveErr(cmd, "issue.watchers.add", err)
	}

	// Capture pre-state when readback is requested so we can populate
	// `was_already_watching` in the envelope .
	watcherSvc := jira.NewWatcherService(client)
	var preState *jira.WatchersResponse
	if !args.NoReadback {
		preState, _, _ = watcherSvc.List(cmd.Context(), args.Key)
	}

	if _, err := watcherSvc.Add(cmd.Context(), args.Key, accountID); err != nil {
		return err
	}

	if args.NoReadback {
		return writeEnvelope(cmd, "issue.watchers.add", map[string]any{
			"account_id": accountID,
			"attempted":  true,
		})
	}

	post, resp, err := watcherSvc.List(cmd.Context(), args.Key)
	if err != nil {
		return err
	}
	wasAlready := containsAccount(preState, accountID)
	return writeEnvelopeWithResponse(cmd, "issue.watchers.add", map[string]any{
		"watchers":             watcherListData(post.Watchers),
		"is_watching":          post.IsWatching,
		"watch_count":          post.WatchCount,
		"was_already_watching": wasAlready,
	}, resp)
}

func runWatcherRemove(cmd *cobra.Command, args watcherMutationArgs) error {
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.watchers.remove")
	}
	user := jira.NewUserService(client)

	if args.DryRun {
		accountID, rerr := user.ResolveUser(cmd.Context(), args.UserIdent)
		if rerr != nil {
			return handleResolveErr(cmd, "issue.watchers.remove", rerr)
		}
		return writeEnvelope(cmd, "issue.watchers.remove", map[string]any{
			"key":                 args.Key,
			"account_id_resolved": accountID,
			"dry_run":             true,
		})
	}

	accountID, err := user.ResolveUser(cmd.Context(), args.UserIdent)
	if err != nil {
		return handleResolveErr(cmd, "issue.watchers.remove", err)
	}

	watcherSvc := jira.NewWatcherService(client)
	var preState *jira.WatchersResponse
	if !args.NoReadback {
		preState, _, _ = watcherSvc.List(cmd.Context(), args.Key)
	}

	if _, err := watcherSvc.Remove(cmd.Context(), args.Key, accountID); err != nil {
		return err
	}

	if args.NoReadback {
		return writeEnvelope(cmd, "issue.watchers.remove", map[string]any{
			"account_id": accountID,
			"attempted":  true,
		})
	}

	post, resp, err := watcherSvc.List(cmd.Context(), args.Key)
	if err != nil {
		return err
	}
	wasAlready := containsAccount(preState, accountID)
	return writeEnvelopeWithResponse(cmd, "issue.watchers.remove", map[string]any{
		"watchers":             watcherListData(post.Watchers),
		"is_watching":          post.IsWatching,
		"watch_count":          post.WatchCount,
		"was_already_watching": wasAlready,
	}, resp)
}

// handleResolveErr maps the resolver's (ErrUserNotFound / *AmbiguousUserError /
// transport) error into the right exit-code envelope:
//   - 0 matches  → exit 2 with `errors[].type = not_found`, input echoed
//   - 2+ matches → exit 3 with `errors[].type = validation` and a
//     structured `errors[].candidates: [...]` per envelope-shapes.md.
//     Returns envelopeWrittenError so writeCommandError doesn't overwrite
//     the richer envelope we already emitted.
func handleResolveErr(cmd *cobra.Command, command string, err error) error {
	var ambig *jira.AmbiguousUserError
	if errors.As(err, &ambig) {
		// Route through the central error-envelope builder so the
		// ambiguity failure carries a stable code, meta.exit_code, and
		// the same shape as every other error envelope. cli.MapError
		// recognizes *jira.AmbiguousUserError and flattens its
		// /user/search candidates into errors[].candidates.
		env := cli.ErrorEnvelope(command, err)
		if len(env.Errors) == 1 {
			env.Errors[0].Hint = "Re-run with --user accountId:<id>."
		}
		_ = cli.WriteEnvelope(cmd.OutOrStdout(), env)
		return envelopeWrittenError{inner: fmt.Errorf("validation: %w", err)}
	}
	if errors.Is(err, jira.ErrUserNotFound) {
		return fmt.Errorf("not found: %w", err)
	}
	return err
}

// containsAccount reports whether `pre` already lists the supplied
// accountID. Pre-state may be nil if the readback GET failed before the
// mutation; treat that as "unknown → false" rather than panicking.
func containsAccount(pre *jira.WatchersResponse, accountID string) bool {
	if pre == nil {
		return false
	}
	for _, u := range pre.Watchers {
		if u != nil && u.AccountID != nil && *u.AccountID == accountID {
			return true
		}
	}
	return false
}

// watcherListData converts the raw User slice from the wire into the
// snake-case shape the envelope contract requires.
func watcherListData(users []*jira.User) []map[string]any {
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		entry := map[string]any{
			"account_id":   derefString(u.AccountID),
			"display_name": derefString(u.DisplayName),
		}
		if u.EmailAddress != nil {
			entry["email_address"] = *u.EmailAddress
		}
		out = append(out, entry)
	}
	return out
}

// derefString turns a possibly-nil *string into "", used by the
// envelope projectors that walk pkg/jira's nullable fields.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefInt64 turns a possibly-nil *int64 into 0.
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
