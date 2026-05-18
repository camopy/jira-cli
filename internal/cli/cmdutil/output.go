package cmdutil

import (
	"os"

	"github.com/gechr/x/terminal"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// resolvedOutputMode returns the output mode resolved by PersistentPreRunE
// from the --output flag and terminal/agent detection. It is the single
// source of truth for every command output helper.
func resolvedOutputMode(cmd *cobra.Command) cli.Mode {
	return DetectorFromContext(cmd).Mode
}

// UseCompactOutput reports whether the command runs in compact mode.
func UseCompactOutput(cmd *cobra.Command) bool {
	return resolvedOutputMode(cmd) == cli.ModeCompact
}

// UsePlainOutput reports whether the command runs in a plain (human) or TUI
// output mode.
func UsePlainOutput(cmd *cobra.Command) bool {
	mode := resolvedOutputMode(cmd)
	return mode == cli.ModePlain || mode == cli.ModeTUI
}

// PlainOptionsForCommand builds the plain-renderer option set for a command,
// threading TTY detection, terminal width, and (when available) the active
// profile's base URL.
func PlainOptionsForCommand(cmd *cobra.Command) []cli.PlainOption {
	det := DetectorFromContext(cmd)
	opts := []cli.PlainOption{
		cli.WithPlainTTY(det.IsTTY),
		cli.WithPlainTermWidth(terminal.Width(os.Stdout)),
	}
	if baseURL := plainBaseURL(cmd); baseURL != "" {
		opts = append(opts, cli.WithPlainBaseURL(baseURL))
	}
	return opts
}

// plainBaseURL returns the active profile's base URL for plain-mode link
// rendering, or "" when the config cannot be loaded.
func plainBaseURL(cmd *cobra.Command) string {
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return ""
	}
	return ActiveProfile(cmd, cfg).BaseURL
}
