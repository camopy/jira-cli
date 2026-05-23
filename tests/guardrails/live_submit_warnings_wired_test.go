// MOTIVATION: pipeline warnings being silently dropped on the
// live-submit path is exactly the failure mode the second critique
// caught. Comments in this file are PROVENANCE ONLY and MUST NOT be a
// source of implementation, fixtures, wording, or test logic.
package guardrails

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Every mutation command's live-submit return site MUST pass pipeline
// warnings into the envelope. The cmdutil.WriteEnvelopeWithResponse
// helper has no warnings parameter — using it for a mutation drops
// warnings silently. The right helper is
// cmdutil.WriteEnvelopeWithResponseAndWarnings.
//
// This guard parses every non-test .go file under cmd/jira/ and
// internal/cli/ and asserts that every call to WriteEnvelopeWithResponse
// uses the AndWarnings variant when it sits inside a known-mutation
// command. Scanning both trees keeps the guard live regardless of which
// package a mutation command currently lives in, and the per-function
// found check fails loudly if a move drops one out of the scanned tree.
func TestLiveSubmitMutationsThreadWarnings(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "cmd", "jira"),
		filepath.Join("..", "..", "internal", "cli"),
	}

	// Mutation command function names — every live-submit return inside
	// these MUST go through writeEnvelopeWithResponseAndWarnings.
	// Mutation paths implemented as run* helpers (issueEditWithEditor,
	// runCommentAdd, runCommentEdit) are listed alongside the command
	// builders because they own a live-submit return site too.
	mutationFns := map[string]bool{
		"issueCreateCommand":      true,
		"issueEditCommand":        true,
		"issueEditWithEditor":     true,
		"runCommentAdd":           true,
		"runCommentEdit":          true,
		"issueTransitionCommand":  true,
		"worklogAddCommand":       true,
		"destructiveIssueCommand": true,
	}

	seen := map[string]bool{}
	for _, root := range roots {
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
			if parseErr != nil {
				return fmt.Errorf("parse %s: %w", path, parseErr)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Name == nil {
					return true
				}
				if !mutationFns[fn.Name.Name] {
					return true
				}
				seen[fn.Name.Name] = true
				// Walk the function body looking for WriteEnvelopeWithResponse
				// calls that should be AndWarnings.
				ast.Inspect(fn.Body, func(child ast.Node) bool {
					call, ok := child.(*ast.CallExpr)
					if !ok {
						return true
					}
					if calledName(call) == "WriteEnvelopeWithResponse" {
						pos := fset.Position(call.Pos())
						t.Errorf("%s:%d %q uses WriteEnvelopeWithResponse (no warnings) inside mutation %q — must use WriteEnvelopeWithResponseAndWarnings",
							path, pos.Line, callSnippet(call, fset), fn.Name.Name)
					}
					return true
				})
				return true
			})
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", root, walkErr)
		}
	}

	// Every expected mutation function MUST be found and inspected. A
	// rename or move that drops one out of the scanned tree fails loudly
	// rather than passing silently — the failure mode that previously hid
	// worklog.add when it moved into internal/cli.
	for name := range mutationFns {
		if !seen[name] {
			t.Errorf("mutation command %q not found under cmd/jira or internal/cli — guard cannot see it (moved or renamed?)", name)
		}
	}
}

// calledName returns the function name of a call expression, handling
// both bare calls (foo()) and qualified calls (pkg.Foo()). The helper
// moved into the cmdutil package, so call sites are now selector
// expressions rather than bare identifiers.
func calledName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// callSnippet returns a short rendering of a call expression for error
// messages. The name is enough to locate the line.
func callSnippet(call *ast.CallExpr, _ *token.FileSet) string {
	name := calledName(call)
	if name == "" {
		name = "?"
	}
	return name + "(...)"
}
