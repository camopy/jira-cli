// MOTIVATION: pipeline warnings being silently dropped on the
// live-submit path is exactly the failure mode the second critique
// caught. Comments in this file are PROVENANCE ONLY and MUST NOT be a
// source of implementation, fixtures, wording, or test logic.
package guardrails

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Every mutation command's live-submit return site MUST pass pipeline
// warnings into the envelope. The writeEnvelopeWithResponse helper has
// no warnings parameter — using it for a mutation drops warnings
// silently. The right helper is writeEnvelopeWithResponseAndWarnings.
//
// This guard parses cmd/jira/commands.go and asserts that every call to
// writeEnvelopeWithResponse uses the AndWarnings variant when it sits
// inside a known-mutation command. It pins the call sites so a future
// refactor that drops the warnings argument is caught.
func TestLiveSubmitMutationsThreadWarnings(t *testing.T) {
	path := filepath.Join("..", "..", "cmd", "jira", "commands.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	// Mutation command function names — every live-submit return inside
	// these MUST go through writeEnvelopeWithResponseAndWarnings. The
	// external-editor helper (issueEditWithEditor) is included because
	// it's a mutation path even though it's a helper, not a command
	// builder.
	mutationFns := map[string]bool{
		"issueCreateCommand":      true,
		"issueEditCommand":        true,
		"issueEditWithEditor":     true,
		"issueCommentCommand":     true,
		"issueTransitionCommand":  true,
		"worklogAddCommand":       true,
		"destructiveIssueCommand": true,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			return true
		}
		if !mutationFns[fn.Name.Name] {
			return true
		}
		// Walk the function body looking for writeEnvelopeWithResponse
		// calls that should be AndWarnings.
		ast.Inspect(fn.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == "writeEnvelopeWithResponse" {
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d %q uses writeEnvelopeWithResponse (no warnings) inside mutation %q — must use writeEnvelopeWithResponseAndWarnings",
					path, pos.Line, callSnippet(call, fset), fn.Name.Name)
			}
			return true
		})
		return true
	})
}

// callSnippet returns a short rendering of a call expression for error
// messages. Caller name + first arg should be enough to locate the line.
func callSnippet(call *ast.CallExpr, fset *token.FileSet) string {
	var b strings.Builder
	if id, ok := call.Fun.(*ast.Ident); ok {
		b.WriteString(id.Name)
	}
	b.WriteString("(...)")
	return b.String()
}
