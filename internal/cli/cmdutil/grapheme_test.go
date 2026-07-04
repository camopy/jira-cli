package cmdutil

import "testing"

// graphemeTerminal must recognize the terminals that cluster graphemes
// unconditionally and stay conservative (wcwidth) everywhere else.
func TestGraphemeTerminalSniff(t *testing.T) {
	cases := []struct {
		name        string
		termProgram string
		term        string
		want        bool
	}{
		{"ghostty via TERM_PROGRAM", "ghostty", "xterm-256color", true},
		{"ghostty via TERM", "", "xterm-ghostty", true},
		{"kitty via TERM", "", "xterm-kitty", true},
		{"wezterm", "WezTerm", "xterm-256color", true},
		{"foot", "", "foot-extra", true},
		{"windows terminal", "", "xterm-256color", false},
		{"bare xterm", "", "xterm", false},
		{"nothing set", "", "", false},
		{"apple terminal", "Apple_Terminal", "xterm-256color", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", tc.termProgram)
			t.Setenv("TERM", tc.term)
			if got := graphemeTerminal(); got != tc.want {
				t.Errorf("graphemeTerminal() = %v, want %v", got, tc.want)
			}
		})
	}
}
