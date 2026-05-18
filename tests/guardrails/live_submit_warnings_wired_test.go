// MOTIVATION: pipeline warnings being silently dropped on the
// live-submit path is exactly the failure mode the second critique
// caught. Comments in this file are PROVENANCE ONLY and MUST NOT be a
// source of implementation, fixtures, wording, or test logic.
package guardrails

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
// This guard parses every non-test .go file under cmd/jira/ and asserts
// that every call to WriteEnvelopeWithResponse uses the AndWarnings
// variant when it sits inside a known-mutation command. Scanning the
// whole command directory keeps the guard live regardless of which file
// a mutation command currently lives in.
func TestLiveSubmitMutationsThreadWarnings(t *testing.T) {
	dir := filepath.Join("..", "..", "cmd", "jira")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
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

	sawMutationFn := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				return true
			}
			if !mutationFns[fn.Name.Name] {
				return true
			}
			sawMutationFn = true
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
	}

	// A guard that traverses zero mutation commands is inert. If a
	// rename or move drops every mutation function out of cmd/jira/,
	// fail loudly rather than passing silently.
	if !sawMutationFn {
		t.Fatalf("no mutation command functions found under %s — guard is inert", dir)
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
