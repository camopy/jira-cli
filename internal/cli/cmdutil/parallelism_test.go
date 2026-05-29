package cmdutil

import (
	"strings"
	"testing"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"
)

func TestAddParallelismFlagRegistersLocalFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "read"}
	var parallelism int

	AddParallelismFlag(cmd, &parallelism)

	flag := cmd.Flags().Lookup("parallelism")
	if flag == nil {
		t.Fatal("missing --parallelism flag")
	}
	if cmd.PersistentFlags().Lookup("parallelism") != nil {
		t.Fatal("--parallelism must be local, not persistent")
	}
	if flag.Shorthand != "p" {
		t.Fatalf("--parallelism shorthand = %q, want p", flag.Shorthand)
	}
	if flag.DefValue != "1" || parallelism != 1 {
		t.Fatalf("--parallelism default = flag %q target %d, want 1", flag.DefValue, parallelism)
	}

	meta := clib.FlagMeta(cmd)
	var found bool
	for _, item := range meta {
		if item.Name != "parallelism" {
			continue
		}
		found = true
		if item.Short != "p" || item.Group != "Execution" || item.Placeholder != "N" || item.Terse != "max concurrent requests" {
			t.Fatalf("parallelism metadata = %+v", item)
		}
	}
	if !found {
		t.Fatalf("parallelism missing from clib metadata: %+v", meta)
	}
}

func TestParallelismFlagParsesValidValues(t *testing.T) {
	cmd := &cobra.Command{Use: "read"}
	var parallelism int
	AddParallelismFlag(cmd, &parallelism)

	if err := cmd.Flags().Parse([]string{"-p", "4"}); err != nil {
		t.Fatalf("parse -p 4: %v", err)
	}
	if parallelism != 4 {
		t.Fatalf("parallelism = %d, want 4", parallelism)
	}
}

func TestCommandParallelismReadsLocalFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "read"}
	var parallelism int
	AddParallelismFlag(cmd, &parallelism)

	if got := commandParallelism(cmd); got != 1 {
		t.Fatalf("commandParallelism(default) = %d, want 1", got)
	}
	if err := cmd.Flags().Parse([]string{"--parallelism", "4"}); err != nil {
		t.Fatalf("parse --parallelism 4: %v", err)
	}
	if got := commandParallelism(cmd); got != 4 {
		t.Fatalf("commandParallelism(non-default) = %d, want 4", got)
	}
}

func TestCommandParallelismDefaultsWhenFlagMissing(t *testing.T) {
	cmd := &cobra.Command{Use: "read"}

	if got := commandParallelism(cmd); got != 1 {
		t.Fatalf("commandParallelism(no flag) = %d, want 1", got)
	}
}

func TestParallelismFlagRejectsOutOfRangeValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "below minimum", args: []string{"--parallelism", "0"}},
		{name: "above maximum", args: []string{"--parallelism", "17"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "read"}
			var parallelism int
			AddParallelismFlag(cmd, &parallelism)

			err := cmd.Flags().Parse(tc.args)
			if err == nil {
				t.Fatalf("parse %v succeeded, want range error", tc.args)
			}
			if !strings.Contains(err.Error(), "parallelism must be between 1 and 16") {
				t.Fatalf("parse error = %q, want clear range error", err)
			}
		})
	}
}
