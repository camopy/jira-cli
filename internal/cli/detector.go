package cli

import (
	"fmt"
	"os"

	"github.com/gechr/x/agent"
	"github.com/gechr/x/terminal"
)

// Mode is the concrete rendering the output machinery resolves to, after the
// --output flag and auto-detection are folded together (ResolveOutputMode).
type Mode string

const (
	// ModePlain is the human-facing renderer: styled tables and status lines.
	ModePlain Mode = "plain"
	// ModeTUI is the full-screen dashboard; only the tui command produces it.
	ModeTUI Mode = "tui"
	// ModeJSON is the full JSON envelope (ok/meta/data/warnings/errors).
	ModeJSON Mode = "json"
	// ModeCompact is the JSON data payload without the envelope wrapper.
	ModeCompact Mode = "compact"
)

// OutputMode is the value of the canonical --output flag. It is the ONLY
// output-mode selector: the legacy --json/--compact/--plain/--raw booleans
// are removed and there is no raw REST passthrough mode.
type OutputMode string

const (
	// OutputAuto defers to terminal/agent detection: TTY -> human,
	// non-TTY -> json, detected agent -> compact.
	OutputAuto OutputMode = "auto"
	// OutputHuman forces the rich clog/table renderer.
	OutputHuman OutputMode = "human"
	// OutputJSON forces the full JSON envelope (ok/meta/data/warnings/errors).
	OutputJSON OutputMode = "json"
	// OutputCompact forces the JSON data payload without the envelope
	// wrapper (no ok/meta/warnings/errors).
	OutputCompact OutputMode = "compact"
)

// OutputModeValues lists the accepted --output values for help text and
// shell completion.
var OutputModeValues = []string{"auto", "human", "json", "compact"}

// ParseOutputMode validates a raw --output flag value. An empty string
// resolves to OutputAuto. Any other unrecognized value — including the
// removed "raw"/"plain"/"tui" names — is rejected.
func ParseOutputMode(v string) (OutputMode, error) {
	switch v {
	case "", "auto":
		return OutputAuto, nil
	case "human":
		return OutputHuman, nil
	case "json":
		return OutputJSON, nil
	case "compact":
		return OutputCompact, nil
	default:
		// A typed flag-value error keeps this on the flag_value_invalid
		// code with the offending flag named, consistent with every other
		// flag-value parse failure.
		fe := NewCLIInputError(InputFlagValueInvalid, fmt.Sprintf("invalid --output mode %q: must be one of auto, human, json, compact", v))
		fe.Flag = "output"
		return "", fe
	}
}

// ResolveOutputMode turns the --output flag value plus auto-detection into
// the concrete rendering Mode. An explicit value overrides detection;
// OutputAuto follows the Detection.Mode produced by Detect.
func ResolveOutputMode(out OutputMode, det Detection) Mode {
	switch out {
	case OutputHuman:
		return ModePlain
	case OutputJSON:
		return ModeJSON
	case OutputCompact:
		return ModeCompact
	default: // OutputAuto
		// Detect never produces ModeTUI for ordinary command output, so
		// the detected mode is returned as-is.
		return det.Mode
	}
}

// Detection is the resolved output environment for one invocation: the mode
// auto-detection chose plus the signals it derived it from, seeded into the
// command context by root's PersistentPreRunE.
type Detection struct {
	Mode  Mode
	IsTTY bool
	Agent bool
	// AgentName is the hosting agent's name as reported by x/agent.Detect
	// (e.g. "claude", "codex"), or empty when the agent cannot be named —
	// which can happen even when Agent is true (a bare `AGENT=1` opt-in).
	AgentName string
	// StdinPiped reports that stdin is NOT an interactive terminal (a pipe,
	// redirect, or /dev/null). It is deliberately negative so the zero value
	// means "interactive" — the safe default for code that builds a Detection
	// without a real stdin. Set from the runtime's stdin in PersistentPreRunE.
	StdinPiped bool
}

// Detect resolves the output environment from stdout: a detected agent yields
// compact, a non-TTY yields json, and an interactive terminal yields plain.
// Agent detection is delegated to x/agent: the cross-tool `AGENT`/`AI_AGENT`
// convention first (a falsy value is an explicit opt-out), then per-vendor
// marker variables (`CLAUDECODE`, `CODEX_SANDBOX`, ...).
func Detect(stdout *os.File) Detection {
	d := Detection{
		IsTTY:     terminal.Is(stdout),
		Agent:     agent.Is(),
		AgentName: agent.Detect(),
	}
	switch {
	case d.Agent:
		d.Mode = ModeCompact
	case !d.IsTTY:
		d.Mode = ModeJSON
	default:
		d.Mode = ModePlain
	}
	return d
}

// RequireTTY is Detect with the added precondition that stdout is interactive,
// for commands (the dashboard) that cannot run otherwise. It returns the
// Detection alongside the error so a caller can still inspect the environment.
func RequireTTY(stdout *os.File) (Detection, error) {
	d := Detect(stdout)
	if !d.IsTTY {
		return d, fmt.Errorf("tui requires an interactive terminal")
	}
	return d, nil
}
