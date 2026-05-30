package cmdutil_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
)

// NoInputRequested implies headless mode for a non-interactive session (agent
// or piped stdin) so a scripted mutation never stalls on a prompt, while an
// explicit --no-input always wins. The probe is stdin, not stdout.
func TestNoInputRequested(t *testing.T) {
	for _, tc := range []struct {
		name     string
		det      cli.Detection
		explicit string // "" = flag untouched; "true"/"false" = explicit --no-input
		want     bool
	}{
		{name: "interactive stdin, flag unset", det: cli.Detection{}, want: false},
		{name: "piped stdin implies no-input", det: cli.Detection{StdinPiped: true}, want: true},
		{name: "agent implies no-input", det: cli.Detection{Agent: true}, want: true},
		{name: "explicit --no-input wins over interactive", det: cli.Detection{}, explicit: "true", want: true},
		{name: "explicit --no-input=false wins over piped stdin", det: cli.Detection{StdinPiped: true}, explicit: "false", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "x"}
			cmd.PersistentFlags().Bool("no-input", false, "")
			if tc.explicit != "" {
				if err := cmd.PersistentFlags().Set("no-input", tc.explicit); err != nil {
					t.Fatalf("set no-input: %v", err)
				}
			}
			cmd.SetContext(cmdutil.WithDetector(context.Background(), tc.det))
			if got := cmdutil.NoInputRequested(cmd); got != tc.want {
				t.Fatalf("NoInputRequested() = %v, want %v", got, tc.want)
			}
		})
	}
}
