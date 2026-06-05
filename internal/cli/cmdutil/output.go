package cmdutil

import (
	"os"

	"github.com/gechr/clog"
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
	if IsStructuredAgentCommand(cmd) {
		return false
	}
	mode := resolvedOutputMode(cmd)
	return mode == cli.ModePlain || mode == cli.ModeTUI
}

// UseHumanJSONOutput reports whether the command runs in human mode but
// should still render a JSON document through clog's pretty JSON printer.
func UseHumanJSONOutput(cmd *cobra.Command) bool {
	if !IsStructuredAgentCommand(cmd) {
		return false
	}
	mode := resolvedOutputMode(cmd)
	return mode == cli.ModePlain || mode == cli.ModeTUI
}

// IsStructuredAgentCommand reports whether a command is an agent-facing
// structured metadata endpoint. These commands always print JSON documents;
// even --output=human uses clog's JSON printer rather than key-value logs.
func IsStructuredAgentCommand(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "jira agent schema", "jira agent adf-matrix", "jira agent fieldtypes":
		return true
	default:
		return false
	}
}

// PlainOptionsForCommand builds the plain-renderer option set for a command,
// threading TTY detection, terminal width, execution hints, and (when
// available) the active profile's base URL.
func PlainOptionsForCommand(cmd *cobra.Command) []cli.PlainOption {
	det := DetectorFromContext(cmd)
	opts := []cli.PlainOption{
		cli.WithPlainTTY(det.IsTTY),
		cli.WithPlainTermWidth(terminal.Width(os.Stdout)),
	}
	if debug, _ := cmd.Root().PersistentFlags().GetBool("debug"); debug {
		opts = append(opts, cli.WithPlainDebug(true))
	}
	if parallelism := commandParallelism(cmd); parallelism != defaultParallelism {
		opts = append(opts, cli.WithPlainThreads(parallelism))
	}
	// Load the active config once and derive both the base URL (for link
	// rendering) and the theme from it. On a load error both are simply
	// omitted, leaving the renderer's defaults.
	if cfg, err := config.Load(config.WithPath(ConfigPath(cmd))); err == nil {
		if baseURL := ActiveProfile(cmd, cfg).BaseURL; baseURL != "" {
			opts = append(opts, cli.WithPlainBaseURL(baseURL))
		}
		// The "auto" theme detects the terminal background so hash-based
		// entity colors stay readable. Detection runs only for "auto" users
		// with color enabled — never under --color=never or NO_COLOR, where
		// there is no color to contrast and a terminal query would be pure
		// waste — so the round-trip never costs anyone else.
		if config.IsAutoTheme(cfg.Theme.Name) && !clog.ColorsDisabled() {
			opts = append(opts, cli.WithPlainTheme(config.AutoTheme(os.Stdout)))
		}
	}
	if cmd.Flags().Lookup("columns") != nil {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil && len(columns) > 0 {
			opts = append(opts, cli.WithPlainColumns(columns))
		}
	}
	if cmd.Flags().Lookup("tsv") != nil {
		if tsv, err := cmd.Flags().GetBool("tsv"); err == nil && tsv {
			opts = append(opts, cli.WithPlainTSV(true))
		}
	}
	return opts
}

func commandParallelism(cmd *cobra.Command) int {
	if flag := cmd.Flags().Lookup("parallelism"); flag == nil {
		return defaultParallelism
	}
	value, err := cmd.Flags().GetInt("parallelism")
	if err != nil {
		return defaultParallelism
	}
	return value
}
