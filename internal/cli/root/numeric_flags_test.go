package root

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// numericFlagsFixture mirrors the shape validateNumericFlags sees at run
// time: --timeout / --max-retry-wait live on the root's persistent flags,
// --limit on the invoked subcommand.
func numericFlagsFixture(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	rootCmd := &cobra.Command{Use: "jira"}
	rootCmd.PersistentFlags().Duration("timeout", 0, "")
	rootCmd.PersistentFlags().Duration("max-retry-wait", 30*time.Second, "")
	child := &cobra.Command{Use: "list"}
	child.Flags().Int("limit", 50, "")
	rootCmd.AddCommand(child)
	return rootCmd, child
}

func TestValidateNumericFlagsRejectsNegatives(t *testing.T) {
	cases := []struct {
		name  string
		flag  string
		value string
		want  string
	}{
		{"negative timeout", "timeout", "-10s", "--timeout"},
		{"negative max-retry-wait", "max-retry-wait", "-1s", "--max-retry-wait"},
		{"negative limit", "limit", "-5", "--limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd, child := numericFlagsFixture(t)
			flags := rootCmd.PersistentFlags()
			if tc.flag == "limit" {
				flags = child.Flags()
			}
			if err := flags.Set(tc.flag, tc.value); err != nil {
				t.Fatalf("Set(%s) error = %v", tc.flag, err)
			}
			err := validateNumericFlags(child)
			if err == nil {
				t.Fatalf("validateNumericFlags accepted --%s %s", tc.flag, tc.value)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %s", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "validation:") {
				t.Fatalf("error %q is not validation-classed (exit 3)", err)
			}
		})
	}
}

// Zero is each flag's documented disabled/default sentinel, and a command
// without a --limit flag has nothing to validate.
func TestValidateNumericFlagsAcceptsZeroAndDefaults(t *testing.T) {
	rootCmd, child := numericFlagsFixture(t)
	if err := validateNumericFlags(child); err != nil {
		t.Fatalf("validateNumericFlags(defaults) error = %v", err)
	}
	if err := child.Flags().Set("limit", "0"); err != nil {
		t.Fatalf("Set(limit) error = %v", err)
	}
	if err := rootCmd.PersistentFlags().Set("timeout", "0"); err != nil {
		t.Fatalf("Set(timeout) error = %v", err)
	}
	if err := validateNumericFlags(child); err != nil {
		t.Fatalf("validateNumericFlags(zeros) error = %v", err)
	}
	if err := validateNumericFlags(rootCmd); err != nil {
		t.Fatalf("validateNumericFlags(no --limit flag) error = %v", err)
	}
}
