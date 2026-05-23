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
	"github.com/gechr/clog"
	"github.com/gechr/x/terminal"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/alias"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/runtime"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errCompletionHandled is returned by Execute when a shell-completion
// preflight request was fully serviced. main translates it into a
// zero-exit termination; no command runs. Keeping this as a returned
// sentinel — rather than an os.Exit deep in construction — leaves main
// the sole owner of process exit.
var errCompletionHandled = errors.New("completion request handled")

// newRootCommand builds the bare root *cobra.Command: its metadata,
// persistent flags, groups, help renderer, and PersistentPreRunE/RunE.
// It does NOT attach subcommands or the completion command — that is
// NewRootCommand's job, so the bare root can also seed the completion
// generator before its own completion subcommand exists.
//
// Each call returns an independent command with its own persistent flag
// set: there is no shared process-global command object.
func newRootCommand(rt *runtime.Runtime) *cobra.Command {
	root := &cobra.Command{
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
		// Enable Cobra's command-name typo suggestions. The field defaults
		// to 0 (exact match only); 2 is Cobra's documented sweet spot and
		// the threshold the unknown-command path reads via SuggestionsFor.
		SuggestionsMinimumDistance: 2,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return rootPersistentPreRun(cmd, rt)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rootRun(cmd, rt)
		},
	}

	// Route command IO through the runtime streams so output capture is
	// per-instance: a test (or an embedding caller) supplying buffers via
	// runtime options sees every command write land in those buffers, and
	// two roots in one process never share an output destination.
	root.SetOut(rt.Stdout())
	root.SetErr(rt.Stderr())
	root.SetIn(rt.Stdin())

	// Convert pflag parse failures into typed *cli.CLIInputError values.
	// Set on root, the function is inherited by every subcommand (Cobra
	// walks the parent chain for FlagErrorFunc). clib's cobra helper
	// registers none of its own, so there is nothing to chain through.
	root.SetFlagErrorFunc(newFlagError)

	configureRootFlags(root)
	configureRootGroups(root)
	configureRootHelp(root)

	return root
}

// rootPersistentPreRun resolves logging, color, and output-mode detection
// for an invocation, then seeds cmd.Context() with the detector and a
// fresh credential-warning sink. Output detection reads the runtime
// stdout writer so a redirected stream is detected consistently.
func rootPersistentPreRun(cmd *cobra.Command, rt *runtime.Runtime) error {
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
	logger := clog.With().Logger()

	outputRaw, _ := pf.GetString("output")
	outputMode, err := cli.ParseOutputMode(outputRaw)
	if err != nil {
		return err
	}
	det := detectOutput(rt)
	det.Mode = cli.ResolveOutputMode(outputMode, det)
	interactive, _ := pf.GetBool("interactive")
	if interactive {
		det.Mode = cli.ModeTUI
	}
	// cmd.Context() is always populated: Execute drives the tree through
	// ExecuteContextC, which seeds every command's context from main's
	// signal-aware root context.
	ctx := cmd.Context()
	ctx = cmdutil.WithDetector(ctx, det)
	// Install a fresh credential-warning sink for this command invocation
	// so a legacy-keyring-fallback warning is scoped to the command that
	// produced it and cannot bleed into another.
	ctx = cmdutil.WithCredentialWarnSink(ctx)
	ctx = logger.WithContext(ctx)
	cmd.SetContext(ctx)
	event := logger.Debug().Str("mode", string(det.Mode))
	if det.AgentName != "" {
		event.Str("agent", det.AgentName)
	} else {
		event.Str("agent", "null")
	}
	event.Msg("output detection")

	// Check required flags here, before Cobra's own ValidateRequiredFlags
	// runs (PersistentPreRunE precedes it in the command lifecycle), so a
	// missing required flag leaves the command layer as a typed error
	// rather than Cobra's untyped "required flag(s) ... not set" string.
	if missing := missingRequiredFlags(cmd); len(missing) > 0 {
		return requiredFlagError(missing)
	}
	return nil
}

// rootRun is the bare `jira` behavior: launch the dashboard when
// interactive, print help for a human TTY, or emit JSON discovery for a
// pipe/agent.
func rootRun(cmd *cobra.Command, rt *runtime.Runtime) error {
	det := cmdutil.DetectorFromContext(cmd)
	interactive, _ := cmd.Root().PersistentFlags().GetBool("interactive")
	if interactive && !runtimeStdoutIsTTY(rt) {
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
}

// detectOutput resolves the output detection for the runtime's stdout
// writer. When stdout is an *os.File (the production path) detection
// inspects that descriptor; otherwise (a buffer in tests or an embedding
// caller) it falls back to the process stdout so env-driven agent
// detection still applies.
func detectOutput(rt *runtime.Runtime) cli.Detection {
	if f, ok := rt.Stdout().(*os.File); ok {
		return cli.Detect(f)
	}
	return cli.Detect(os.Stdout)
}

// runtimeStdoutIsTTY reports whether the runtime stdout writer is an
// interactive terminal. A non-*os.File writer (test buffer) is never a
// TTY.
func runtimeStdoutIsTTY(rt *runtime.Runtime) bool {
	f, ok := rt.Stdout().(*os.File)
	return ok && terminal.Is(f)
}

// NewRootCommand builds a fully assembled root command for the given
// runtime: the bare root plus every command family and the shell
// completion command. Each call yields an independent command tree with
// its own flag set and IO wiring — no process-global command state.
func NewRootCommand(rt *runtime.Runtime) *cobra.Command {
	root := newRootCommand(rt)
	root.AddCommand(clib.CompletionCommand(root, func() *complete.Generator {
		return completionGenerator(root)
	}))
	registerCommands(root)
	// Wrap every command's positional-argument validator so a count
	// failure surfaces as a typed *cli.CLIInputError. Done after the tree
	// is fully assembled so it reaches every command.
	retypeArgValidators(root)
	return root
}

// completionGenerator builds the clib completion generator for a root
// command. Shared by the `completion` subcommand and the preflight path.
func completionGenerator(root *cobra.Command) *complete.Generator {
	gen := complete.NewGenerator("jira", complete.WithOrder(complete.OrderKeep)).FromFlags(rootCompletionFlagMeta(root))
	gen.Subs = clib.Subcommands(root)
	return gen
}

func rootCompletionFlagMeta(root *cobra.Command) []complete.FlagMeta {
	meta := clib.FlagMeta(root)
	for i := range meta {
		switch meta[i].Name {
		case "config", "profile":
			meta[i].Forward = true
		}
	}
	return meta
}

// configureRootFlags declares the global persistent flags on root. Each
// call installs a fresh, independent flag set.
func configureRootFlags(root *cobra.Command) {
	pf := root.PersistentFlags()
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
	clib.Extend(pf.Lookup("no-input"), clib.FlagExtra{Group: "Runtime", Terse: "disable prompts"})
	clib.Extend(pf.Lookup("color"), clib.FlagExtra{
		Group:       "Output",
		Enum:        []string{"auto", "always", "never"},
		EnumTerse:   []string{"detect terminal", "force color", "no color"},
		EnumDefault: "auto",
		Terse:       "color mode",
	})
	clib.Extend(pf.Lookup("adf-strict"), clib.FlagExtra{Group: "ADF", Terse: "reject lossy ADF"})
	clib.Extend(pf.Lookup("adf-best-effort"), clib.FlagExtra{Group: "ADF", Terse: "allow lossy ADF"})

	root.MarkFlagsMutuallyExclusive("adf-strict", "adf-best-effort")
}

// configureRootGroups declares the help command groups on root.
func configureRootGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: "dashboard", Title: "Dashboard"},
		&cobra.Group{ID: "resources", Title: "Jira Resources"},
		&cobra.Group{ID: "configuration", Title: "Configuration"},
		&cobra.Group{ID: "agent", Title: "Agent Discovery"},
	)
}

// configureRootHelp installs the themed clib help renderer on root.
func configureRootHelp(root *cobra.Command) {
	root.SetHelpFunc(clib.HelpFunc(cmdutil.NewHelpRenderer(), cmdutil.StandardHelpSections))
}

// Execute builds the runtime and root command for a process invocation,
// services any shell-completion preflight, applies the optional
// whole-invocation timeout, and runs the command tree against ctx.
//
// ctx is the root context main owns (signal-aware via signal.NotifyContext).
// Execute never calls os.Exit: a completion preflight that was fully
// handled is reported back to main as errCompletionHandled.
func Execute(ctx context.Context) error {
	rt, err := runtime.New()
	if err != nil {
		return err
	}
	// NewRootCommand builds a static command tree; it deliberately takes
	// no context. The per-command PersistentPreRunE derives its context
	// from cmd.Context() at run time — cobra seeds that from the ctx
	// passed to ExecuteContextC below. contextcheck cannot see that
	// deferred handoff and flags the construction call as a missing
	// context thread.
	root := NewRootCommand(rt) //nolint:contextcheck // context flows via ExecuteContextC, not construction

	if handled, err := handleCompletionPreflight(root); err != nil {
		writeCommandError(ctx, root, err)
		return err
	} else if handled {
		return errCompletionHandled
	}

	args, err := alias.ExpandAliasArgs(root, os.Args[1:])
	if err != nil {
		writeCommandError(ctx, root, err)
		return err
	}
	if args != nil {
		root.SetArgs(args)
	}
	// Reject an unknown top-level command before execution so it carries
	// the stable command_unknown code. Cobra's own unknown-command error
	// is an untyped string; here it becomes a typed *cli.CLIInputError
	// with "did you mean" candidates from Cobra's suggestion logic.
	effectiveArgs := args
	if effectiveArgs == nil {
		effectiveArgs = os.Args[1:]
	}
	if _, _, ferr := root.Find(effectiveArgs); ferr != nil {
		cerr := unknownCommandError(root, firstPositional(root, effectiveArgs))
		writeCommandError(ctx, root, cerr)
		return cerr
	}
	// Optional whole-invocation deadline. Derive it here, where Execute
	// already owns the root context, with a local defer cancel(): no
	// cancel func is parked in a package var, and the context is never
	// stored on a runtime struct. The deadline spans command execution
	// (alias expansion above is flag/file work with no network I/O; the
	// timeout's purpose is bounding the request).
	if timeout := timeoutFromArgs(args); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	// ExecuteContext seeds cmd.Context() with the (optionally
	// deadline-bounded) root context so every RunE that calls
	// cmd.Context() inherits cancellation.
	cmd, err := root.ExecuteContextC(ctx)
	if err != nil {
		if cmd != nil {
			writeCommandError(cmd.Context(), cmd, err) //nolint:contextcheck // use command context seeded by PersistentPreRunE
			return err
		}
		writeCommandError(ctx, root, err)
		return err
	}
	return nil
}

// handleCompletionPreflight services a clib `--@complete` preflight
// request against root. It returns handled=true when the request was a
// completion request that was fully serviced; the caller must then
// terminate without running a command. Returning the handled signal
// instead of calling os.Exit keeps process termination with main.
func handleCompletionPreflight(root *cobra.Command) (bool, error) {
	flags, positional, ok := clib.Preflight()
	if !ok {
		return false, nil
	}
	gen := completionGenerator(root)
	globals := startup.GlobalsFromArgs(os.Args[1:])
	if forwarded := startup.GlobalsFromArgs(positional); forwarded.ConfigPath != "" || forwarded.Profile != "" {
		if forwarded.ConfigPath != "" {
			globals.ConfigPath = forwarded.ConfigPath
		}
		if forwarded.Profile != "" {
			globals.Profile = forwarded.Profile
		}
	}
	handled, err := flags.Handle(gen, completionHandler(globals), complete.WithArgs(positional))
	if err != nil {
		return false, err
	}
	return handled, nil
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

func writeCommandError(ctx context.Context, cmd *cobra.Command, err error) {
	if err == nil {
		return
	}
	// Failures always emit a clog diagnostic on stderr.
	logger := clog.Ctx(ctx)
	if logger == clog.Default {
		logger = clog.New(clog.NewOutput(cmd.ErrOrStderr(), clog.ColorAuto))
	}
	logger.Error().Err(err).Send()
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
