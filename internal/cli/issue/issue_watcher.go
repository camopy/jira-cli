package issue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
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
	var dryRun, noReadback, validateRemote bool
	var parallelism int
	cmd := &cobra.Command{
		Use:         "watch KEY...",
		Short:       "Start watching an issue (alias for watchers add --user me)",
		Args:        cobra.MinimumNArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runWatcherAddKeys(cmd, keys, parallelism, watcherMutationArgs{
				UserIdent: "me", DryRun: dryRun, NoReadback: noReadback,
				ValidateRemote: validateRemote,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Local preview only — does not contact Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	cmd.Flags().BoolVar(&validateRemote, "validate-remote", false, "Resolve --user against Jira (read-only); use with --dry-run")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendWatcherValidationFlags(cmd.Flags())
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

// issueUnwatchCommand wires `jira issue unwatch KEY` — equivalent to
// `watchers remove --user me`.
func issueUnwatchCommand() *cobra.Command {
	var dryRun, noReadback, validateRemote bool
	var parallelism int
	cmd := &cobra.Command{
		Use:         "unwatch KEY...",
		Short:       "Stop watching an issue (alias for watchers remove --user me)",
		Args:        cobra.MinimumNArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runWatcherRemoveKeys(cmd, keys, parallelism, watcherMutationArgs{
				UserIdent: "me", DryRun: dryRun, NoReadback: noReadback,
				ValidateRemote: validateRemote,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Local preview only — does not contact Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	cmd.Flags().BoolVar(&validateRemote, "validate-remote", false, "Resolve --user against Jira (read-only); use with --dry-run")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendWatcherValidationFlags(cmd.Flags())
	cmdutil.AddParallelismFlag(cmd, &parallelism)
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
	// ValidateRemote opts a dry-run into resolving --user against Jira's
	// read-only /myself + /user search endpoints. Plain --dry-run is
	// local preview only and never contacts Jira.
	ValidateRemote bool
}

// ----- list -----------------------------------------------------------------

func watcherListCommand() *cobra.Command {
	var parallelism int
	cmd := &cobra.Command{
		Use:           "list KEY...",
		Short:         "List watchers on an issue",
		Args:          cobra.MinimumNArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.watchers.list")
			}
			service := jira.NewWatcherService(client)
			if len(keys) == 1 {
				watchers, resp, err := service.List(cmd.Context(), keys[0])
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.watchers.list", watcherListEnvelopeData(watchers), resp)
			}
			results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (*jira.WatchersResponse, error) {
				watchers, _, err := service.List(ctx, key)
				return watchers, err
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.watchers.list", results, func(_ string, watchers *jira.WatchersResponse) any {
				return watcherListEnvelopeData(watchers)
			})
		},
	}
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func watcherListEnvelopeData(watchers *jira.WatchersResponse) map[string]any {
	if watchers == nil {
		return map[string]any{
			"watchers":    []map[string]any{},
			"is_watching": false,
			"watch_count": 0,
		}
	}
	return map[string]any{
		"watchers":    watcherListData(watchers.Watchers),
		"is_watching": watchers.IsWatching,
		"watch_count": watchers.WatchCount,
	}
}

// ----- add ------------------------------------------------------------------

func watcherAddCommand() *cobra.Command {
	var user string
	var dryRun, noReadback, validateRemote bool
	var parallelism int
	cmd := &cobra.Command{
		Use:           "add KEY...",
		Short:         "Add a watcher to an issue",
		Args:          cobra.MinimumNArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if user == "" {
				return fmt.Errorf("validation: --user is required (me / accountId:<id> / email)")
			}
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runWatcherAddKeys(cmd, keys, parallelism, watcherMutationArgs{
				UserIdent: user, DryRun: dryRun, NoReadback: noReadback,
				ValidateRemote: validateRemote,
			})
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "User identifier (me / accountId:<id> / email)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Local preview only — does not contact Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	cmd.Flags().BoolVar(&validateRemote, "validate-remote", false, "Resolve --user against Jira (read-only); use with --dry-run")
	cmdutil.ExtendWatcherUserFlag(cmd.Flags())
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendWatcherValidationFlags(cmd.Flags())
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

// ----- remove ---------------------------------------------------------------

func watcherRemoveCommand() *cobra.Command {
	var user string
	var dryRun, noReadback, validateRemote bool
	var parallelism int
	cmd := &cobra.Command{
		Use:           "remove KEY...",
		Short:         "Remove a watcher from an issue",
		Args:          cobra.MinimumNArgs(1),
		Annotations:   issueKeyArg,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if user == "" {
				return fmt.Errorf("validation: --user is required (me / accountId:<id> / email)")
			}
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			return runWatcherRemoveKeys(cmd, keys, parallelism, watcherMutationArgs{
				UserIdent: user, DryRun: dryRun, NoReadback: noReadback,
				ValidateRemote: validateRemote,
			})
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "User identifier (me / accountId:<id> / email)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Local preview only — does not contact Jira")
	cmd.Flags().BoolVar(&noReadback, "no-readback", false, "Skip the post-mutation GET")
	cmd.Flags().BoolVar(&validateRemote, "validate-remote", false, "Resolve --user against Jira (read-only); use with --dry-run")
	cmdutil.ExtendWatcherUserFlag(cmd.Flags())
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	cmdutil.ExtendWatcherValidationFlags(cmd.Flags())
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

// ----- shared add/remove drivers -------------------------------------------

func runWatcherAddKeys(cmd *cobra.Command, keys []string, parallelism int, args watcherMutationArgs) error {
	if len(keys) == 1 {
		args.Key = keys[0]
		return runWatcherAdd(cmd, args)
	}
	return runWatcherMutationMany(cmd, "issue.watchers.add", keys, parallelism, args, watcherMutationAdd)
}

func runWatcherRemoveKeys(cmd *cobra.Command, keys []string, parallelism int, args watcherMutationArgs) error {
	if len(keys) == 1 {
		args.Key = keys[0]
		return runWatcherRemove(cmd, args)
	}
	return runWatcherMutationMany(cmd, "issue.watchers.remove", keys, parallelism, args, watcherMutationRemove)
}

type watcherMutationKind int

const (
	watcherMutationAdd watcherMutationKind = iota
	watcherMutationRemove
)

func runWatcherMutationMany(
	cmd *cobra.Command,
	command string,
	keys []string,
	parallelism int,
	args watcherMutationArgs,
	kind watcherMutationKind,
) error {
	if args.DryRun {
		return watcherDryRunPreviewMany(cmd, command, keys, args)
	}
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for %s", command)
	}
	accountID, err := jira.NewUserService(client).ResolveUser(cmd.Context(), args.UserIdent)
	if err != nil {
		return handleResolveErr(cmd, command, err)
	}
	watcherSvc := jira.NewWatcherService(client)
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		perKey := args
		perKey.Key = key
		switch kind {
		case watcherMutationAdd:
			return watcherAddData(ctx, watcherSvc, accountID, perKey)
		case watcherMutationRemove:
			return watcherRemoveData(ctx, watcherSvc, accountID, perKey)
		default:
			return nil, fmt.Errorf("unknown watcher mutation kind")
		}
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, command, results, func(_ string, data map[string]any) any {
		return data
	})
}

func watcherDryRunPreviewMany(cmd *cobra.Command, command string, keys []string, args watcherMutationArgs) error {
	accountID := ""
	userResolved := false
	if args.ValidateRemote {
		client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("jira base URL is required for %s --validate-remote", command)
		}
		userSvc := jira.NewUserService(client)
		if id, ok := accountIDFromIdentifier(args.UserIdent); ok {
			accountID, err = userSvc.ResolveAccountID(cmd.Context(), id)
		} else {
			accountID, err = userSvc.ResolveUser(cmd.Context(), args.UserIdent)
		}
		if err != nil {
			return handleResolveErr(cmd, command, err)
		}
		userResolved = true
	} else if id, ok := localResolveUser(cmd, args.UserIdent); ok {
		accountID = id
		userResolved = true
	}
	results := make([]cmdutil.KeyResult[map[string]any], len(keys))
	for i, key := range keys {
		data := map[string]any{
			"key":           key,
			"user":          args.UserIdent,
			"dry_run":       true,
			"user_resolved": userResolved,
		}
		if accountID != "" {
			data["account_id_resolved"] = accountID
		}
		results[i] = cmdutil.KeyResult[map[string]any]{Key: key, Value: data}
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, command, results, func(_ string, data map[string]any) any {
		return data
	})
}

func runWatcherAdd(cmd *cobra.Command, args watcherMutationArgs) error {
	if args.DryRun {
		return watcherDryRunPreview(cmd, "issue.watchers.add", args)
	}

	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.watchers.add")
	}
	user := jira.NewUserService(client)

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
	data, err := watcherAddData(cmd.Context(), watcherSvc, accountID, args)
	if err != nil {
		return err
	}
	return cmdutil.WriteEnvelope(cmd, "issue.watchers.add", data)
}

func watcherAddData(ctx context.Context, watcherSvc jira.WatcherService, accountID string, args watcherMutationArgs) (map[string]any, error) {
	var preState *jira.WatchersResponse
	if !args.NoReadback {
		preState, _, _ = watcherSvc.List(ctx, args.Key)
	}

	if _, err := watcherSvc.Add(ctx, args.Key, accountID); err != nil {
		return nil, err
	}

	if args.NoReadback {
		return map[string]any{
			"account_id": accountID,
			"attempted":  true,
		}, nil
	}

	post, _, err := watcherSvc.List(ctx, args.Key)
	if err != nil {
		return nil, err
	}
	wasAlready := containsAccount(preState, accountID)
	return map[string]any{
		"watchers":             watcherListData(post.Watchers),
		"is_watching":          post.IsWatching,
		"watch_count":          post.WatchCount,
		"was_already_watching": wasAlready,
	}, nil
}

func runWatcherRemove(cmd *cobra.Command, args watcherMutationArgs) error {
	if args.DryRun {
		return watcherDryRunPreview(cmd, "issue.watchers.remove", args)
	}

	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.watchers.remove")
	}
	user := jira.NewUserService(client)

	accountID, err := user.ResolveUser(cmd.Context(), args.UserIdent)
	if err != nil {
		return handleResolveErr(cmd, "issue.watchers.remove", err)
	}

	watcherSvc := jira.NewWatcherService(client)
	data, err := watcherRemoveData(cmd.Context(), watcherSvc, accountID, args)
	if err != nil {
		return err
	}
	return cmdutil.WriteEnvelope(cmd, "issue.watchers.remove", data)
}

func watcherRemoveData(ctx context.Context, watcherSvc jira.WatcherService, accountID string, args watcherMutationArgs) (map[string]any, error) {
	var preState *jira.WatchersResponse
	if !args.NoReadback {
		preState, _, _ = watcherSvc.List(ctx, args.Key)
	}

	if _, err := watcherSvc.Remove(ctx, args.Key, accountID); err != nil {
		return nil, err
	}

	if args.NoReadback {
		return map[string]any{
			"account_id": accountID,
			"attempted":  true,
		}, nil
	}

	post, _, err := watcherSvc.List(ctx, args.Key)
	if err != nil {
		return nil, err
	}
	wasAlready := containsAccount(preState, accountID)
	return map[string]any{
		"watchers":             watcherListData(post.Watchers),
		"is_watching":          post.IsWatching,
		"watch_count":          post.WatchCount,
		"was_already_watching": wasAlready,
	}, nil
}

// localResolveUser resolves a watcher --user identifier WITHOUT
// contacting Jira. It succeeds only for identifiers that are derivable
// from local state:
//
//   - "accountId:<id>" → the id, parsed from the prefix.
//   - "me" / "@me"     → the active profile's AccountID, when set.
//
// A bare name or email genuinely requires a /user/search hop, so it
// returns ("", false): the caller must report it unresolved rather than
// fabricate a result or sneak in a live call.
func localResolveUser(cmd *cobra.Command, ident string) (string, bool) {
	v := strings.TrimSpace(ident)
	if id, ok := strings.CutPrefix(v, "accountId:"); ok {
		id = strings.TrimSpace(id)
		return id, id != ""
	}
	switch strings.ToLower(v) {
	case "me", "@me":
		profile, err := cmdutil.ProfileForCommand(cmd)
		if err != nil {
			return "", false
		}
		return profile.AccountID, profile.AccountID != ""
	default:
		return "", false
	}
}

// watcherDryRunPreview renders the local preview for watch / unwatch.
//
// Plain --dry-run is local-only: it contacts Jira for nothing. An
// identifier that is locally derivable (accountId:<id>, or "me" when the
// profile carries an AccountID) is resolved here and reported in
// `account_id_resolved`; a bare name or email cannot be resolved without
// a live call, so it is echoed back with `user_resolved:false` and no
// `account_id_resolved`. --validate-remote opts into resolving the user
// against Jira's read-only /myself + /user search endpoints — never a
// watcher POST/DELETE. The `user_resolved` flag keeps the preview honest
// about whether resolution actually ran.
func watcherDryRunPreview(cmd *cobra.Command, command string, args watcherMutationArgs) error {
	data := map[string]any{
		"key":           args.Key,
		"user":          args.UserIdent,
		"dry_run":       true,
		"user_resolved": false,
	}
	if !args.ValidateRemote {
		if id, ok := localResolveUser(cmd, args.UserIdent); ok {
			data["account_id_resolved"] = id
			data["user_resolved"] = true
		}
		return cmdutil.WriteEnvelope(cmd, command, data)
	}
	// --validate-remote: resolve via the read-only user service. No
	// mutation is ever issued on this path.
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for %s --validate-remote", command)
	}
	userSvc := jira.NewUserService(client)
	var (
		accountID string
		rerr      error
	)
	if id, ok := accountIDFromIdentifier(args.UserIdent); ok {
		accountID, rerr = userSvc.ResolveAccountID(cmd.Context(), id)
	} else {
		accountID, rerr = userSvc.ResolveUser(cmd.Context(), args.UserIdent)
	}
	if rerr != nil {
		return handleResolveErr(cmd, command, rerr)
	}
	data["account_id_resolved"] = accountID
	data["user_resolved"] = true
	return cmdutil.WriteEnvelope(cmd, command, data)
}

func accountIDFromIdentifier(ident string) (string, bool) {
	id, ok := strings.CutPrefix(strings.TrimSpace(ident), "accountId:")
	if !ok {
		return "", false
	}
	id = strings.TrimSpace(id)
	return id, id != ""
}

// handleResolveErr maps the resolver's (ErrUserNotFound / *AmbiguousUserError /
// transport) error into the right exit-code envelope:
//   - 0 matches  → exit 2 with `errors[].type = not_found`, input echoed
//   - 2+ matches → exit 3 with `errors[].type = validation` and a
//     structured `errors[].candidates: [...]` per envelope-shapes.md.
//     Returns a cmdutil.EnvelopeWrittenError so the central error writer
//     doesn't overwrite the richer envelope we already emitted.
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
		_ = cli.WriteEnvelope(cmd.ErrOrStderr(), env)
		return cmdutil.EnvelopeWritten(fmt.Errorf("validation: %w", err))
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
