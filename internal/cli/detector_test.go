package cli

import "testing"

func TestDetectAgentWithMatchesGHStyleSignals(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want AgentName
	}{
		{name: "clean", env: map[string]string{}, want: ""},
		{name: "generic AI_AGENT", env: map[string]string{"AI_AGENT": "some-agent"}, want: "some-agent"},
		{name: "invalid AI_AGENT falls through", env: map[string]string{"AI_AGENT": "bad agent", "GEMINI_CLI": "1"}, want: agentGeminiCLI},
		{name: "amp before claude", env: map[string]string{"AGENT": "amp", "CLAUDECODE": "1"}, want: agentAmp},
		{name: "codex sandbox", env: map[string]string{"CODEX_SANDBOX": "danger-full-access"}, want: agentCodex},
		{name: "codex thread", env: map[string]string{"CODEX_THREAD_ID": "abc"}, want: agentCodex},
		{name: "copilot cli", env: map[string]string{"COPILOT_CLI": "1"}, want: agentCopilotCLI},
		{name: "cursor", env: map[string]string{"CURSOR_TERMINAL": "1"}, want: agentCursor},
		{name: "claude code", env: map[string]string{"CLAUDECODE": "1"}, want: agentClaudeCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAgentWith(func(key string) (string, bool) {
				v, ok := tt.env[key]
				return v, ok
			})
			if got != tt.want {
				t.Fatalf("detectAgentWith() = %q, want %q", got, tt.want)
			}
		})
	}
}
