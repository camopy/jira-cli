package unit

import (
	"strconv"
	"testing"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/boards"
	cachereg "github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/issue"
)

// Commands that read a shared cache resource must take that resource's TTL
// from the registry, not a private hardcoded window — otherwise `jira boards
// list` refetches a boards cache that `jira cache boards` still considers
// fresh, and `jira issue link types` does the same against `jira cache
// linktypes`. This guards that consumption-layer consistency.
func TestConsumerCommandsTrackRegistryTTL(t *testing.T) {
	cases := []struct {
		name     string
		root     *cobra.Command
		path     []string
		resource string
	}{
		{"boards list", boards.NewCommand(), []string{"list"}, "boards"},
		{"issue link types", issue.NewCommand(), []string{"link", "types"}, "linktypes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := findSubcommand(tc.root, tc.path)
			if cmd == nil {
				t.Fatalf("subcommand %v not found", tc.path)
			}
			f := cmd.Flags().Lookup("ttl-minutes")
			if f == nil {
				t.Fatalf("%s has no --ttl-minutes flag", tc.name)
			}
			if want := strconv.Itoa(cachereg.TTLMinutesFor(tc.resource)); f.DefValue != want {
				t.Errorf("%s --ttl-minutes default %s != registry %s for %q",
					tc.name, f.DefValue, want, tc.resource)
			}
		})
	}
}

func findSubcommand(root *cobra.Command, path []string) *cobra.Command {
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}
