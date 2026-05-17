package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// expectedPublicLeaves enumerates every public (non-hidden) leaf command
// path the CLI must expose. A leaf is a command with no subcommands. This
// list is the locked public surface: the per-instance root factory must
// produce exactly these leaves so a behavior-preserving refactor cannot
// silently drop or rename a command.
var expectedPublicLeaves = []string{
	"jira tui",
	"jira me",
	"jira version",
}

// collectLeaves walks the Cobra tree rooted at cmd and returns the dotted
// command path of every public leaf (available, no available children).
func collectLeaves(cmd *cobra.Command) []string {
	var leaves []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		var children []*cobra.Command
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() {
				children = append(children, sub)
			}
		}
		if len(children) == 0 {
			leaves = append(leaves, c.CommandPath())
			return
		}
		for _, sub := range children {
			walk(sub)
		}
	}
	walk(cmd)
	return leaves
}

// TestRootCommandHasStablePublicLeaves asserts the per-instance root
// factory produces a Cobra tree containing every public leaf the CLI
// promises. It checks containment rather than exact equality so adding a
// command later does not break the test, but removing one does.
func TestRootCommandHasStablePublicLeaves(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	leaves := collectLeaves(root)
	have := map[string]bool{}
	for _, l := range leaves {
		have[l] = true
	}
	for _, want := range expectedPublicLeaves {
		if !have[want] {
			t.Errorf("root command tree missing public leaf %q; have %v", want, leaves)
		}
	}
}

// TestRootCommandAttachesGlobalFlagsOnce asserts every global persistent
// flag is attached exactly once on the per-instance root, and that two
// independently constructed roots own distinct flag sets (no shared
// process-global flag state).
func TestRootCommandAttachesGlobalFlagsOnce(t *testing.T) {
	globalFlags := []string{
		"profile", "config", "output", "interactive", "debug",
		"no-input", "timeout", "color", "adf-strict", "adf-best-effort",
	}

	rootA, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest A: %v", err)
	}
	rootB, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest B: %v", err)
	}

	for _, name := range globalFlags {
		fa := rootA.PersistentFlags().Lookup(name)
		if fa == nil {
			t.Errorf("root A missing global flag --%s", name)
			continue
		}
		fb := rootB.PersistentFlags().Lookup(name)
		if fb == nil {
			t.Errorf("root B missing global flag --%s", name)
			continue
		}
		if fa == fb {
			t.Errorf("flag --%s is shared between two roots: per-instance construction must not reuse the same *pflag.Flag", name)
		}
	}
}

// TestRootCommandPreservesRequiredHiddenAliases asserts the required
// top-level commands survive the refactor: `completion` must be present.
func TestRootCommandPreservesRequiredHiddenAliases(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	required := []string{"completion"}
	for _, name := range required {
		found := false
		for _, sub := range root.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("root command missing required alias/command %q", name)
		}
	}
}

// TestNewRootCommandDoesNotCallOsExit is a static check: the library
// command-construction path (NewRootCommand and the command factories it
// calls) must never call os.Exit. Only main.go and the completion
// preflight in main may terminate the process. A NewRootCommand that
// calls os.Exit is untestable and unusable as a library.
func TestNewRootCommandDoesNotCallOsExit(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/jira: %v", err)
	}
	// main.go legitimately owns the process exit. Every other file in
	// package main is library construction code and must not exit.
	allowed := map[string]bool{"main.go": true}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if allowed[name] {
			continue
		}
		// This walk inspects call expressions only; scope objects are
		// not needed, so skip object resolution.
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "os" && sel.Sel.Name == "Exit" {
				t.Errorf("%s calls os.Exit; only main.go may exit the process", name)
			}
			return true
		})
	}
}
