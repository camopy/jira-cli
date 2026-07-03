// Package update implements the `jira update` cobra command, which
// self-updates a release-archive binary in place and points Scoop and
// go-install binaries at the command their channel manages updates with.
package update

import (
	"context"
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/selfupdate"
	"github.com/matcra587/jira-cli/internal/version"
)

// updater is the slice of selfupdate.Updater the command consumes, seamed so
// tests can stub release resolution and the install itself.
type updater interface {
	Latest(ctx context.Context) (string, error)
	Update(ctx context.Context) error
}

// Test seams: package vars so unit tests can pin a channel and stub the
// network/filesystem work without a real release or a writable install dir.
var (
	detectChannel  = selfupdate.Detect
	newUpdater     = func(ch selfupdate.Channel) (updater, error) { return selfupdate.NewUpdater(ch) }
	currentVersion = version.Version
)

// NewCommand returns the `update` command: self-update for release-archive
// installs, channel-specific update hints for Scoop and go-install binaries.
func NewCommand() *cobra.Command {
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update jira-cli to the latest release",
		Long: "Update the running binary to the latest release. The install channel is detected " +
			"from the binary itself: a Homebrew install runs `brew upgrade` through its tap, a " +
			"release-archive install is replaced in place (checksum-verified, rollback-safe), " +
			"while Scoop, mise, and `go install` binaries are owned by their installer — the " +
			"command reports the exact update command to run instead of touching them.\n\n" +
			"`--dry-run` resolves the channel and latest release and reports whether an update is " +
			"available without changing anything. A live self-replace requires `--force` in " +
			"headless, agent, or `--no-input` mode; an interactive terminal is prompted. A " +
			"from-source build has no update channel and fails with guidance.",
		Example: `$ jira update

# Report the channel and latest release without changing anything
$ jira update --dry-run

# Non-interactive (agent / script) update
$ jira update --force --output=json`,
		GroupID: "configuration",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, dryRun, force)
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Report channel and latest release without updating")
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm replacing the running binary")
	return cmd
}

func run(cmd *cobra.Command, dryRun, force bool) error {
	channel := detectChannel()
	current := currentVersion()

	switch channel {
	case selfupdate.ChannelScoop:
		return writeManaged(cmd, channel, current, selfupdate.ScoopHint, dryRun)
	case selfupdate.ChannelMise:
		return writeManaged(cmd, channel, current, selfupdate.MiseHint, dryRun)
	case selfupdate.ChannelGoInstall:
		return writeManaged(cmd, channel, current, selfupdate.GoInstallHint, dryRun)
	case selfupdate.ChannelBrew, selfupdate.ChannelArchive:
	case selfupdate.ChannelUnknown:
		fallthrough
	default:
		// Unknown includes the zero Channel: a value that never came from
		// Detect must fail closed here, not reach the self-update path.
		return fmt.Errorf(
			"cannot determine the install channel of this binary (a from-source build has no update channel); "+
				"reinstall via Homebrew, Scoop, mise, a GitHub release archive, or `%s`",
			selfupdate.GoInstallHint,
		)
	}

	// Self-replacing the binary is a destructive-ish local mutation: headless,
	// agent, and --no-input callers must consent with --force before any
	// network work happens. --dry-run never mutates, so it stays open.
	det := cmdutil.DetectorFromContext(cmd)
	headless := !det.IsTTY || det.Agent || cmdutil.NoInputRequested(cmd)
	if !dryRun && !force && headless {
		return errors.New("update requires --force in headless / agent / --no-input mode")
	}

	up, err := newUpdater(channel)
	if err != nil {
		return err
	}
	var latest string
	if err := cmdutil.Spin(cmd, "update.check", func(ctx context.Context) error {
		var e error
		latest, e = up.Latest(ctx)
		return e
	}); err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}
	available := selfupdate.UpdateAvailable(current, latest)

	data := map[string]any{
		"channel":          string(channel),
		"current":          current,
		"latest":           latest,
		"update_available": available,
		"updated":          false,
		"dry_run":          dryRun,
	}
	if dryRun || !available {
		return cmdutil.WriteEnvelope(cmd, "update", data)
	}

	if !force {
		// Headless callers were rejected above, so this is a TTY human:
		// confirm before replacing their binary.
		ok, err := confirmUpdate(cmd, current, latest)
		if err != nil {
			return err
		}
		if !ok {
			return cli.NewPromptError(cli.PromptAborted, "update", nil)
		}
	}

	// clive owns the progress spinners and the old→new report line on stderr;
	// wrapping it in cmdutil.Spin would nest spinners.
	if err := up.Update(cmd.Context()); err != nil { //nolint:gocritic // not a Jira call; clive renders its own spinner
		return err
	}
	data["updated"] = true
	return cmdutil.WriteEnvelope(cmd, "update", data)
}

// writeManaged emits the envelope for a channel whose installer owns updates:
// jira reports the command to run and changes nothing.
func writeManaged(cmd *cobra.Command, channel selfupdate.Channel, current, hint string, dryRun bool) error {
	return cmdutil.WriteEnvelope(cmd, "update", map[string]any{
		"channel": string(channel),
		"current": current,
		"managed": true,
		"hint":    hint,
		"updated": false,
		"dry_run": dryRun,
	})
}

// confirmUpdate prompts a TTY human before the binary is replaced. Mirrors
// the destructive-mutation confirmation semantics: Esc/Ctrl-C declines,
// context cancellation keeps its identity as a typed prompt error.
func confirmUpdate(cmd *cobra.Command, current, latest string) (bool, error) {
	confirmed := false
	confirm := huh.NewConfirm().
		Title(fmt.Sprintf("Update jira-cli %s -> %s?", current, latest)).
		Description("The running binary will be replaced in place.").
		Affirmative("Yes, update").
		Negative("Cancel").
		Value(&confirmed)
	form := huh.NewForm(huh.NewGroup(confirm))
	if err := form.RunWithContext(cmd.Context()); err != nil {
		switch {
		case errors.Is(err, huh.ErrUserAborted):
			return false, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return false, cli.NewPromptError(cli.PromptCanceled, "update confirmation", err)
		default:
			return false, cli.NewPromptError(cli.PromptUnavailable, "update confirmation", err)
		}
	}
	return confirmed, nil
}
