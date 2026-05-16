package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/clib/complete"
	"github.com/gechr/clib/help"
	"github.com/gechr/clib/theme"
	"github.com/gechr/clog"
	"github.com/gechr/x/terminal"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type contextKey string

const (
	detectorKey contextKey = "detector"
	// credentialWarnSinkKey carries the per-command credential-warning sink.
	credentialWarnSinkKey contextKey = "credential-warn-sink"
)

var rootCmd = &cobra.Command{
	Use:   "jira",
	Short: "Jira CLI",
	Long:  "TUI-first, agent-ready CLI for Jira developer workflows.",
	Example: `# Launch the persistent dashboard
	$ jira -i

	# List issues as structured JSON
	$ jira issue list --output=json

# Run a saved JQL query
$ jira search saved my-open-bugs`,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Name() == "completion" || (cmd.Parent() != nil && cmd.Parent().Name() == "completion") {
			return nil
		}

		pf := cmd.Root().PersistentFlags()
		debug, _ := pf.GetBool("debug")
		clog.SetEnvPrefix("JIRA")
		clog.SetVerbose(debug)

		if colorMode, _ := pf.GetString("color"); colorMode != "" {
			switch colorMode {
			case "auto":
				clog.SetOutput(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorAuto))
			case "always":
				clog.SetOutput(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorAlways))
				clog.SetColorMode(clog.ColorAlways)
			case "never":
				clog.SetOutput(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorNever))
				clog.SetColorMode(clog.ColorNever)
			default:
				return fmt.Errorf("invalid color mode %q: must be \"auto\", \"always\" or \"never\"", colorMode)
			}
		} else {
			clog.SetOutput(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorAuto))
		}

		outputRaw, _ := pf.GetString("output")
		outputMode, err := cli.ParseOutputMode(outputRaw)
		if err != nil {
			return err
		}
		det := cli.Detect(os.Stdout)
		det.Mode = cli.ResolveOutputMode(outputMode, det)
		interactive, _ := pf.GetBool("interactive")
		if interactive {
			det.Mode = cli.ModeTUI
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = context.WithValue(ctx, detectorKey, det)
		// Install a fresh credential-warning sink for this command invocation
		// so a legacy-keyring-fallback warning is scoped to the command that
		// produced it and cannot bleed into another.
		ctx = withCredentialWarnSink(ctx)
		cmd.SetContext(ctx)
		event := clog.Debug().Str("mode", string(det.Mode))
		if det.AgentName != "" {
			event.Str("agent", det.AgentName)
		} else {
			event.Str("agent", "null")
		}
		event.Msg("output detection")
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		det := DetectorFromContext(cmd)
		interactive, _ := cmd.Root().PersistentFlags().GetBool("interactive")
		if interactive && !terminal.Is(os.Stdout) {
			return fmt.Errorf("tui requires an interactive terminal")
		}
		if interactive && det.Mode == cli.ModeTUI {
			_, err := tuiRun(cmd)
			return err
		}
		// Bare `jira` behavior:
		//   - TTY (human): print help so users get an immediate command list
		//     instead of a wall of JSON.
		//   - non-TTY / agent: emit JSON discovery so pipes and AI agents
		//     keep working (locked contract).
		// Agents wanting structured discovery in TTY can opt in via
		// `jira agent schema [--compact]`.
		if det.IsTTY && det.Mode == cli.ModePlain {
			return cmd.Help()
		}
		return writeSchema(cmd)
	},
}

func Execute(ctx context.Context) error {
	if err := setup(); err != nil {
		writeCommandError(rootCmd, err)
		return err
	}
	args, err := expandAliasArgs(rootCmd, os.Args[1:])
	if err != nil {
		writeCommandError(rootCmd, err)
		return err
	}
	if args != nil {
		rootCmd.SetArgs(args)
	}
	// Optional whole-invocation deadline. Derive it here, where Execute
	// already owns the root context, with a local defer cancel(): no
	// cancel func is parked in a package var, and the context is never
	// stored on a runtime struct. The deadline spans command execution
	// (setup() and alias expansion above are flag/file work with no
	// network I/O; the timeout's purpose is bounding the request).
	if timeout := timeoutFromArgs(args); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	// ExecuteContext seeds cmd.Context() with the (optionally
	// deadline-bounded) root context so every RunE that calls
	// cmd.Context() inherits cancellation.
	cmd, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		writeCommandError(cmd, err)
		return err
	}
	return nil
}

// timeoutFromArgs extracts the --timeout duration from the resolved
// argv. It parses a throwaway flag set that tolerates every other
// (unknown) flag, so the value is available before cobra runs. args nil
// means "use os.Args[1:]" — the same fallback cobra applies when SetArgs
// was not called.
func timeoutFromArgs(args []string) time.Duration {
	if args == nil {
		args = os.Args[1:]
	}
	fs := pflag.NewFlagSet("timeout-probe", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.Usage = func() {}
	fs.SetOutput(io.Discard)
	timeout := fs.Duration("timeout", 0, "")
	if err := fs.Parse(args); err != nil {
		return 0
	}
	return *timeout
}

func writeCommandError(cmd *cobra.Command, err error) {
	if err == nil {
		return
	}
	// Failures always emit a clog diagnostic on stderr.
	clog.New(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorAuto)).Error().Err(err).Send()
	// Some RunEs (e.g. watcher add ambiguous-resolution) write a richer
	// envelope to stdout themselves (with structured candidates etc.) and
	// signal that with an envelopeWritten wrapper. Avoid double-writing.
	var ew envelopeWrittenError
	if errors.As(err, &ew) {
		return
	}
	// When --json (or --compact) was requested, stdout must also carry a
	// parseable envelope so machine consumers don't see 0 bytes on the JSON
	// path.  We write the envelope AFTER the stderr clog line so stderr
	// consumers still see the human-readable message first.
	if jsonEnvelopeRequested(cmd) {
		// best-effort: the clog stderr line already carries the diagnostic; if
		// the envelope write itself fails (broken stdout pipe), there's nothing
		// useful we can do.
		_ = writeErrorEnvelopeToStdout(cmd, err)
	}
}

// envelopeWrittenError is a wrapper a RunE can return to tell
// writeCommandError "I already emitted a structured envelope to stdout —
// don't overwrite it." outputErrorFor still unwraps to the inner error
// for exit-code classification.
type envelopeWrittenError struct{ inner error }

func (e envelopeWrittenError) Error() string { return e.inner.Error() }
func (e envelopeWrittenError) Unwrap() error { return e.inner }

// jsonEnvelopeRequested returns true when the resolved output mode is a
// machine mode (json or compact), so a command failure still emits a
// parseable error envelope on stdout.
//
// It inspects the --output flag and the env-detector directly so it works
// even before PersistentPreRunE has run (e.g. when cobra fires a flag
// validation error). Detect() reads env vars and the stdout fd directly —
// no cmd.Context() setup is required.
func jsonEnvelopeRequested(cmd *cobra.Command) bool {
	pf := cmd.Root().PersistentFlags()
	outputRaw, _ := pf.GetString("output")
	mode, err := cli.ParseOutputMode(outputRaw)
	if err != nil {
		// An invalid --output value is itself a failure; emit the error
		// envelope so the machine consumer sees a parseable result.
		return true
	}
	det := cli.Detect(os.Stdout)
	resolved := cli.ResolveOutputMode(mode, det)
	return resolved == cli.ModeJSON || resolved == cli.ModeCompact
}

// writeErrorEnvelopeToStdout emits a lean failure envelope carrying the
// error in the errors[] array to cmd's stdout. Used only on the
// json/compact/agent-detected path so machine consumers always have
// parseable output even when the command fails before or after RunE.
//
// This helper deliberately bypasses the compact branching used by the
// success path: errors must stay a parseable JSON envelope regardless of
// the output mode, so a failed command is never invisible to an agent.
func writeErrorEnvelopeToStdout(cmd *cobra.Command, err error) error {
	// Use cobra's CommandPath() so nested sub-commands get the full dotted name
	// (e.g. "issue.create") instead of the hand-walked single-level fallback.
	command := strings.TrimPrefix(cmd.CommandPath(), "jira ")
	command = strings.ReplaceAll(command, " ", ".")
	if command == "" || command == "jira" {
		command = "error"
	}
	env := cli.ErrorEnvelope(command, err)
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

func exitCodeForError(err error) int {
	return cli.ExitCode(outputErrorFor(err))
}

// outputErrorFor maps any command error onto the structured cli.Error
// shape. It delegates entirely to the central cli.MapError mapper, which
// uses errors.As for every typed error — credential, Jira API,
// rate-limit, and the command-local board-validation wrapper (via the
// cli.ValidationCandidatesError interface). There is no command-local
// special case: every error envelope is built one way.
func outputErrorFor(err error) cli.Error {
	return cli.MapError(err)
}

func setup() error {
	rootCmd.AddCommand(clib.CompletionCommand(rootCmd, func() *complete.Generator {
		gen := complete.NewGenerator("jira", complete.WithOrder(complete.OrderKeep)).FromFlags(clib.FlagMeta(rootCmd))
		gen.Subs = clib.Subcommands(rootCmd)
		return gen
	}))

	flags, positional, ok := clib.Preflight()
	if ok {
		gen := complete.NewGenerator("jira", complete.WithOrder(complete.OrderKeep)).FromFlags(clib.FlagMeta(rootCmd))
		gen.Subs = clib.Subcommands(rootCmd)
		handled, err := flags.Handle(gen, completionHandler(), complete.WithArgs(positional))
		if err != nil {
			return err
		}
		if handled {
			os.Exit(0) // completion preflight must terminate after handling
		}
	}
	return nil
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringP("profile", "p", "", "Jira profile name")
	pf.StringP("config", "c", "", "Config file path")
	pf.String("output", "auto", "Output mode: auto, human, json, or compact "+
		"(compact is the JSON data payload without the envelope — no ok/meta/warnings/errors)")
	pf.BoolP("interactive", "i", false, "Launch persistent dashboard from root command")
	pf.BoolP("debug", "d", false, "Enable debug output")
	pf.Bool("no-input", false, "Disable interactive prompts (mandatory for headless / agent invocation)")
	pf.Duration("timeout", 0, "Whole-invocation deadline (e.g. 30s, 2m); 0 disables it")
	pf.String("color", "auto", `Color mode: "auto", "always", or "never"`)
	// ADF strict/best-effort selection — mutually exclusive;
	// internal/cli/adfmode reads them ahead of env/profile/default.
	pf.Bool("adf-strict", false, "Treat lossy ADF conversions as errors")
	pf.Bool("adf-best-effort", false, "Allow lossy ADF conversions with structured warnings")

	clib.Extend(pf.Lookup("profile"), clib.FlagExtra{
		Group:       "Configuration",
		Placeholder: "NAME",
		Complete:    "predictor=profile",
		Terse:       "profile name",
	})
	clib.Extend(pf.Lookup("config"), clib.FlagExtra{
		Group:       "Configuration",
		Placeholder: "PATH",
		Hint:        "file",
		Terse:       "config file path",
	})
	clib.Extend(pf.Lookup("output"), clib.FlagExtra{
		Group:       "Output",
		Placeholder: "MODE",
		Enum:        cli.OutputModeValues,
		EnumTerse:   []string{"detect terminal", "rich text", "JSON envelope", "JSON data only"},
		EnumDefault: "auto",
		Terse:       "output mode",
	})
	clib.Extend(pf.Lookup("timeout"), clib.FlagExtra{
		Group:       "Configuration",
		Placeholder: "DURATION",
		Terse:       "invocation deadline",
	})
	clib.Extend(pf.Lookup("interactive"), clib.FlagExtra{Group: "Dashboard", Terse: "launch dashboard"})
	clib.Extend(pf.Lookup("debug"), clib.FlagExtra{Group: "Output", Terse: "debug output"})
	clib.Extend(pf.Lookup("color"), clib.FlagExtra{
		Group:       "Output",
		Enum:        []string{"auto", "always", "never"},
		EnumTerse:   []string{"detect terminal", "force color", "no color"},
		EnumDefault: "auto",
		Terse:       "color mode",
	})

	rootCmd.MarkFlagsMutuallyExclusive("adf-strict", "adf-best-effort")

	rootCmd.AddGroup(
		&cobra.Group{ID: "dashboard", Title: "Dashboard"},
		&cobra.Group{ID: "resources", Title: "Jira Resources"},
		&cobra.Group{ID: "configuration", Title: "Configuration"},
		&cobra.Group{ID: "agent", Title: "Agent Discovery"},
	)

	theme.SetEnvPrefix("JIRA")
	th := theme.Default().With(
		theme.WithEnumStyle(theme.EnumStyleHighlightBoth),
		theme.WithHelpRepeatEllipsisEnabled(true),
	)
	renderer := help.NewRenderer(th)
	rootCmd.SetHelpFunc(clib.HelpFunc(renderer, clib.SectionsWithOptions(clib.WithSubcommandOptional())))

	registerCommands(rootCmd)
}

func DetectorFromContext(cmd *cobra.Command) cli.Detection {
	v, _ := cmd.Context().Value(detectorKey).(cli.Detection)
	return v
}
