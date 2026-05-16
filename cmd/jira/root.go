package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/matcra587/jira-cli/pkg/jira"
	"github.com/spf13/cobra"
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
	$ jira issue list --json

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

		forcedJSON, _ := pf.GetBool("json")
		det := cli.Detect(os.Stdout, forcedJSON)
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
		return writeSchema(cmd, false)
	},
}

func Execute() error {
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
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		writeCommandError(cmd, err)
		return err
	}
	return nil
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

// jsonEnvelopeRequested returns true when the caller has opted in to structured
// JSON output, either explicitly via --json / --compact flags or implicitly
// because an agent env-var (CLAUDE_CODE, CURSOR_AGENT, etc.) was detected and
// defaulted the session to ModeCompact.
//
// It inspects PersistentFlags and the env-detector directly so it works even
// before PersistentPreRunE has run (e.g. when cobra fires a mutex-flag
// validation error).  The detector reads env vars and the stdout fd directly —
// no cmd.Context() setup is required.
func jsonEnvelopeRequested(cmd *cobra.Command) bool {
	pf := cmd.Root().PersistentFlags()
	if v, _ := pf.GetBool("json"); v {
		return true
	}
	if v, _ := pf.GetBool("compact"); v {
		return true
	}
	// No explicit flag — check whether an agent env-var has defaulted the
	// session to ModeCompact.  Detect() is safe to call here because it only
	// reads os.Environ() and checks whether stdout is a TTY; neither requires
	// context nor any prior cobra setup.
	det := cli.Detect(os.Stdout, false)
	return det.Agent
}

// writeErrorEnvelopeToStdout emits a minimal JSON envelope carrying the error
// in the errors[] array to cmd's stdout.  Used only on the --json / --compact
// / agent-detected path so machine consumers always have parseable output even
// when the command fails before or after RunE.
//
// This helper deliberately bypasses the compact/plain/raw branching used by
// the success path (writeEnvelopeWithWarnings).  Errors must stay parseable
// JSON regardless of mode flags, so the helper is intentionally not
// consolidated with the success-path writer.
func writeErrorEnvelopeToStdout(cmd *cobra.Command, err error) error {
	cliErr := outputErrorFor(err)
	// Use cobra's CommandPath() so nested sub-commands get the full dotted name
	// (e.g. "issue.create") instead of the hand-walked single-level fallback.
	command := strings.TrimPrefix(cmd.CommandPath(), "jira ")
	command = strings.ReplaceAll(command, " ", ".")
	if command == "" || command == "jira" {
		command = "error"
	}
	env := cli.Envelope{
		Meta: cli.Meta{
			Command:   command,
			Profile:   profileForEnvelope(cmd),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: cli.NewRequestID(),
		},
		Data:     map[string]any{},
		Errors:   []cli.Error{cliErr},
		Warnings: []cli.Warning{},
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

func exitCodeForError(err error) int {
	return cli.ExitCode(outputErrorFor(err))
}

func outputErrorFor(err error) cli.Error {
	var apiErr *jira.APIError
	if errors.As(err, &apiErr) {
		return cli.NewError(cli.ErrorType(apiErr.Type), apiErr.Message)
	}
	// Typed "validation" wrapper (board ambiguity / default_board
	// missing) — bypasses the substring classifier so messages
	// containing "not found" still map to exit 3 (validation).
	var bve boardValidationError
	if errors.As(err, &bve) {
		out := cli.NewError(cli.ErrorTypeValidation, bve.Error())
		if cands := bve.BoardCandidates(); len(cands) > 0 {
			out.Candidates = cands
		}
		return out
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "unsupported auth type"):
		return cli.NewError(cli.ErrorTypeValidation, msg)
	case strings.Contains(lower, "credential") || strings.Contains(lower, "auth"):
		return cli.NewError(cli.ErrorTypeAuth, msg)
	case strings.Contains(lower, "not found"):
		return cli.NewError(cli.ErrorTypeNotFound, msg)
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many"):
		return cli.NewError(cli.ErrorTypeRateLimit, msg)
	case strings.Contains(lower, "server"):
		return cli.NewError(cli.ErrorTypeServer, msg)
	default:
		return cli.NewError(cli.ErrorTypeValidation, msg)
	}
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
	pf.Bool("json", false, "Force structured JSON output")
	pf.Bool("compact", false, "Emit compact jq-friendly JSON")
	pf.Bool("plain", false, "Emit clog rich text output")
	pf.Bool("raw", false, "Emit Jira REST-native JSON")
	pf.BoolP("interactive", "i", false, "Launch persistent dashboard from root command")
	pf.BoolP("debug", "d", false, "Enable debug output")
	pf.Bool("no-input", false, "Disable interactive prompts (mandatory for headless / agent invocation)")
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
	clib.Extend(pf.Lookup("json"), clib.FlagExtra{Group: "Output", Terse: "structured JSON"})
	clib.Extend(pf.Lookup("compact"), clib.FlagExtra{Group: "Output", Terse: "compact JSON"})
	clib.Extend(pf.Lookup("plain"), clib.FlagExtra{Group: "Output", Terse: "clog rich text"})
	clib.Extend(pf.Lookup("raw"), clib.FlagExtra{Group: "Output", Terse: "Jira REST JSON"})
	clib.Extend(pf.Lookup("interactive"), clib.FlagExtra{Group: "Dashboard", Terse: "launch dashboard"})
	clib.Extend(pf.Lookup("debug"), clib.FlagExtra{Group: "Output", Terse: "debug output"})
	clib.Extend(pf.Lookup("color"), clib.FlagExtra{
		Group:       "Output",
		Enum:        []string{"auto", "always", "never"},
		EnumTerse:   []string{"detect terminal", "force color", "no color"},
		EnumDefault: "auto",
		Terse:       "color mode",
	})

	rootCmd.MarkFlagsMutuallyExclusive("json", "compact", "plain", "raw")
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
