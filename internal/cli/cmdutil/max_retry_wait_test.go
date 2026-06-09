package cmdutil

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// newRetryWaitCmd returns a root command carrying the --max-retry-wait
// persistent flag, optionally pre-set to mimic an explicit user value.
func newRetryWaitCmd(t *testing.T, explicit string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "root"}
	cmd.PersistentFlags().Duration("max-retry-wait", DefaultMaxRetryWait, "")
	if explicit != "" {
		if err := cmd.PersistentFlags().Set("max-retry-wait", explicit); err != nil {
			t.Fatalf("set --max-retry-wait=%s: %v", explicit, err)
		}
	}
	return cmd
}

func TestMaxRetryWaitFor(t *testing.T) {
	cases := []struct {
		name     string
		explicit string // --max-retry-wait value, "" = not passed
		env      string // JIRA_MAX_RETRY_WAIT value, "" = unset
		want     time.Duration
	}{
		{"default when nothing set", "", "", DefaultMaxRetryWait},
		{"explicit flag wins", "5s", "", 5 * time.Second},
		{"env when no flag", "", "10s", 10 * time.Second},
		{"flag beats env", "5s", "10s", 5 * time.Second},
		{"explicit zero disables", "0s", "", 0},
		{"env zero disables", "", "0s", 0},
		{"negative flag clamps to zero", "-5s", "", 0},
		{"unparseable env falls back to default", "", "soon", DefaultMaxRetryWait},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("JIRA_MAX_RETRY_WAIT", tc.env)
			} else {
				t.Setenv("JIRA_MAX_RETRY_WAIT", "")
			}
			cmd := newRetryWaitCmd(t, tc.explicit)
			if got := MaxRetryWaitFor(cmd); got != tc.want {
				t.Fatalf("MaxRetryWaitFor() = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestMaxRetryWaitForNilCommand guards the defensive nil path: a resolver
// that panics on a nil command would crash callers that build a client
// without one.
func TestMaxRetryWaitForNilCommand(t *testing.T) {
	t.Setenv("JIRA_MAX_RETRY_WAIT", "")
	if got := MaxRetryWaitFor(nil); got != DefaultMaxRetryWait {
		t.Fatalf("MaxRetryWaitFor(nil) = %s, want default %s", got, DefaultMaxRetryWait)
	}
}
