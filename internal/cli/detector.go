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

func Detect(stdout *os.File, forceJSON bool) Detection {
	isTTY := terminal.Is(stdout)
	agent, name := detectAgent()
	d := Detection{
		IsTTY:     isTTY,
		Agent:     agent,
		AgentName: name,
	}
	switch {
	case forceJSON:
		d.Mode = ModeJSON
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
	d := Detect(stdout, false)
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
