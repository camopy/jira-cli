package cmdutil

import (
	"os"
	"strings"

	clibtheme "github.com/gechr/clib/theme"
	"github.com/gechr/clog"
	clogtheme "github.com/gechr/clog/theme"
	"github.com/gechr/x/terminal"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// HumanJSONPrintTheme resolves clog's print theme for human-mode JSON so its
// syntax highlighting follows the same light/dark decision as hash-based entity
// colors. It mirrors the resolution in PlainOptionsForCommand exactly —
// JIRA_THEME wins, the "auto" theme detects the terminal background — so
// highlighted JSON and entity colors never disagree about the background.
func HumanJSONPrintTheme(cmd *cobra.Command) *clogtheme.Theme {
	th := config.DefaultTheme()
	if cfg, err := config.Load(config.WithPath(ConfigPath(cmd))); err == nil {
		if config.IsAutoTheme(cfg.Theme.Name) && !clog.ColorsDisabled() {
			th = config.AutoTheme(os.Stdout)
		}
	}
	if th.Background == clibtheme.BackgroundLight {
		return clogtheme.Light()
	}
	return clogtheme.Dark()
}

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
	case "jira agent schema", "jira agent adf-matrix", "jira agent fieldtypes",
		"jira adf convert", "jira adf render":
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
		cli.WithPlainGraphemeWidth(det.IsTTY && graphemeTerminal()),
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

// graphemeTerminal reports whether the attached terminal measures text in
// grapheme clusters (mode 2027) rather than per-rune wcwidth: it draws a
// ZWJ emoji sequence as one 2-cell glyph and a VS16 emoji as two cells, so
// tables aligned with the wrong method drift on rows containing them.
// Detection is an environment sniff of the terminals known to cluster
// unconditionally (Ghostty, kitty, WezTerm, foot) — right for the
// direct-attach case and conservatively wcwidth over SSH hops that strip
// the variables. A DECRQM 2027 query would be exact; env detection avoids
// a terminal round-trip on every command.
func graphemeTerminal() bool {
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "ghostty", "wezterm", "kitty":
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	for _, prefix := range []string{"xterm-ghostty", "xterm-kitty", "foot"} {
		if strings.HasPrefix(term, prefix) {
			return true
		}
	}
	return false
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
