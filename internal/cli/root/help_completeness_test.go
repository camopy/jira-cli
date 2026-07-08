package root

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestEveryCommandHasHelpText enforces the commands.md contract that every
// command defines Short, Long, and Example — parents included. Help renders
// through clib and agents read Long via the schema, so a Short-only command
// (the shape cmdutil.GroupCommand produces before help text is added) leaves
// both a human and an agent without the "what and why" for that node.
//
// cobra's auto-generated help/completion commands and any hidden command are
// exempt: they are not part of the authored surface.
func TestEveryCommandHasHelpText(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			path := sub.CommandPath()
			if strings.TrimSpace(sub.Short) == "" {
				t.Errorf("%s: missing Short", path)
			}
			if strings.TrimSpace(sub.Long) == "" {
				t.Errorf("%s: missing Long", path)
			}
			if strings.TrimSpace(sub.Example) == "" {
				t.Errorf("%s: missing Example", path)
			}
			walk(sub)
		}
	}
	walk(root)
}
