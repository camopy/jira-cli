package cli

import "testing"

func TestParseOutputModeAcceptsCanonicalValues(t *testing.T) {
	tests := []struct {
		in   string
		want OutputMode
	}{
		{"auto", OutputAuto},
		{"human", OutputHuman},
		{"json", OutputJSON},
		{"compact", OutputCompact},
		{"", OutputAuto},
	}
	for _, tt := range tests {
		got, err := ParseOutputMode(tt.in)
		if err != nil {
			t.Fatalf("ParseOutputMode(%q) error = %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseOutputMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseOutputModeRejectsRemovedAndUnknownValues(t *testing.T) {
	for _, in := range []string{"raw", "plain", "tui", "garbage"} {
		if _, err := ParseOutputMode(in); err == nil {
			t.Fatalf("ParseOutputMode(%q) should reject the value", in)
		}
	}
}

// ResolveOutputMode turns the --output flag value plus the auto-detection
// into a concrete machine/human mode. auto must follow detection; an
// explicit value must override it.
func TestResolveOutputModeAutoFollowsDetection(t *testing.T) {
	tty := Detection{Mode: ModePlain, IsTTY: true}
	nonTTY := Detection{Mode: ModeJSON}
	agent := Detection{Mode: ModeCompact, Agent: true, AgentName: "claude"}

	if got := ResolveOutputMode(OutputAuto, tty); got != ModePlain {
		t.Fatalf("auto+TTY = %q, want plain", got)
	}
	if got := ResolveOutputMode(OutputAuto, nonTTY); got != ModeJSON {
		t.Fatalf("auto+non-TTY = %q, want json", got)
	}
	if got := ResolveOutputMode(OutputAuto, agent); got != ModeCompact {
		t.Fatalf("auto+agent = %q, want compact", got)
	}
}

func TestResolveOutputModeExplicitOverridesDetection(t *testing.T) {
	agent := Detection{Mode: ModeCompact, Agent: true}
	if got := ResolveOutputMode(OutputHuman, agent); got != ModePlain {
		t.Fatalf("explicit human = %q, want plain", got)
	}
	if got := ResolveOutputMode(OutputJSON, agent); got != ModeJSON {
		t.Fatalf("explicit json = %q, want json", got)
	}
	tty := Detection{Mode: ModePlain, IsTTY: true}
	if got := ResolveOutputMode(OutputCompact, tty); got != ModeCompact {
		t.Fatalf("explicit compact = %q, want compact", got)
	}
}
