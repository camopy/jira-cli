package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gechr/x/terminal"
)

type Mode string

const (
	ModePlain   Mode = "plain"
	ModeTUI     Mode = "tui"
	ModeJSON    Mode = "json"
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
		return "", fmt.Errorf("invalid --output mode %q: must be one of auto, human, json, compact", v)
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

type Detection struct {
	Mode      Mode
	IsTTY     bool
	Agent     bool
	AgentName string
}

type AgentName string

const (
	agentAmp        AgentName = "amp"
	agentClaudeCode AgentName = "claude-code"
	agentCodex      AgentName = "codex"
	agentCopilotCLI AgentName = "copilot-cli"
	agentCursor     AgentName = "cursor"
	agentGeminiCLI  AgentName = "gemini-cli"
	agentOpencode   AgentName = "opencode"
)

var validAgentName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func Detect(stdout *os.File) Detection {
	isTTY := terminal.Is(stdout)
	agent, name := detectAgent()
	d := Detection{
		IsTTY:     isTTY,
		Agent:     agent,
		AgentName: name,
	}
	switch {
	case agent:
		d.Mode = ModeCompact
	case !isTTY:
		d.Mode = ModeJSON
	default:
		d.Mode = ModePlain
	}
	return d
}

func RequireTTY(stdout *os.File) (Detection, error) {
	d := Detect(stdout)
	if !d.IsTTY {
		return d, fmt.Errorf("tui requires an interactive terminal")
	}
	return d, nil
}

func detectAgent() (bool, string) {
	name := detectAgentWith(os.LookupEnv)
	return name != "", string(name)
}

func detectAgentWith(lookup func(string) (string, bool)) AgentName {
	isSet := func(key string) bool {
		v, ok := lookup(key)
		return ok && v != "" && truthy(v)
	}
	valueOf := func(key string) string {
		v, _ := lookup(key)
		return v
	}

	if v, ok := lookup("AI_AGENT"); ok && validAgentName.MatchString(v) {
		return AgentName(v)
	}
	if valueOf("AGENT") == "amp" {
		return agentAmp
	}
	if isSet("CODEX_SANDBOX") || isSet("CODEX_CI") || isSet("CODEX_THREAD_ID") || isSet("CODEX") || isSet("OPENAI_CODEX") {
		return agentCodex
	}
	if isSet("GEMINI_CLI") {
		return agentGeminiCLI
	}
	if isSet("COPILOT_CLI") || isSet("COPILOT") || isSet("GITHUB_COPILOT") {
		return agentCopilotCLI
	}
	if isSet("OPENCODE") {
		return agentOpencode
	}
	if isSet("CURSOR_TERMINAL") || isSet("CURSOR_AGENT") {
		return agentCursor
	}
	if isSet("CLAUDECODE") || isSet("CLAUDE_CODE") {
		return agentClaudeCode
	}
	return ""
}

func truthy(v string) bool {
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
