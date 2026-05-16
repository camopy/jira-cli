package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// rootPersistentFlagNames is the set of flags defined on the root command's
// persistent flag set. Any subcommand that defines a local flag with one of
// these names shadows the inherited flag: cobra resolves cmd.Flags() lookups
// to the local copy, so `jira --no-input issue create` sets the root flag
// while the handler reads the unset local one. The two must never collide.
func rootPersistentFlagNames(root *cobra.Command) map[string]bool {
	names := map[string]bool{}
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		names[f.Name] = true
	})
	return names
}

// TestNoSubcommandShadowsRootPersistentFlag walks every command in the tree
// and fails if a subcommand declares a local flag whose name collides with a
// root persistent flag. Such a collision splits the effective flag value
// across two flag objects.
func TestNoSubcommandShadowsRootPersistentFlag(t *testing.T) {
	rootNames := rootPersistentFlagNames(rootCmd)

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd != rootCmd {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				// Only local flags shadow; inherited flags reuse the same
				// *pflag.Flag object and are fine.
				if cmd.LocalFlags().Lookup(f.Name) == nil {
					return
				}
				if rootNames[f.Name] {
					t.Errorf("command %q defines local flag --%s, shadowing the root persistent flag", cmd.CommandPath(), f.Name)
				}
			})
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}
