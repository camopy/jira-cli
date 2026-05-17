// MOTIVATION: command-family extraction moves implementations into
// internal/cli/<domain> packages over several passes. Without an
// enforced contract, an extracted package can quietly reintroduce the
// process-global anti-patterns the refactor exists to remove — calling
// os.Exit, storing a context.Context for later use, or writing to
// os.Stdout instead of the command stream. These guards pin the
// extraction rules so every future domain package is checked the moment
// it lands. Comments in this file are PROVENANCE ONLY and MUST NOT be a
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

// cliPackageRoot is the directory future command-domain packages live
// under. The runtime package is the dependency boundary, not a command
// package, and is exempt from the command-stream / context checks.
const cliPackageRoot = "../../internal/cli"

// commandPackageExemptDirs are directories under internal/cli that are
// not command-domain packages and so are not bound by the extraction
// rules. The os.Exit ban still applies tree-wide.
var commandPackageExemptDirs = map[string]bool{
	"runtime": true,
}

// cliGoFiles returns every non-test .go file under internal/cli, keyed
// by the immediate sub-package directory name.
func cliGoFiles(t *testing.T) map[string][]string {
	t.Helper()
	files := map[string][]string{}
	err := filepath.Walk(cliPackageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(cliPackageRoot, path)
		if relErr != nil {
			return relErr
		}
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = ""
		} else {
			dir = strings.SplitN(filepath.ToSlash(dir), "/", 2)[0]
		}
		files[dir] = append(files[dir], path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/cli: %v", err)
	}
	return files
}

// parseGo parses a Go source file into an AST.
func parseGo(t *testing.T, path string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, file
}

// TestCommandPackagesAvoidProcessExit asserts no package under
// internal/cli calls os.Exit. Process termination is the binary shell's
// job (cmd/jira/main.go); a command package that exits cannot be tested
// or embedded.
func TestCommandPackagesAvoidProcessExit(t *testing.T) {
	for dir, paths := range cliGoFiles(t) {
		for _, path := range paths {
			fset, file := parseGo(t, path)
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if isSelectorCall(call, "os", "Exit") {
					pos := fset.Position(call.Pos())
					t.Errorf("%s:%d: package internal/cli/%s calls os.Exit; only the binary shell may exit the process", path, pos.Line, dir)
				}
				return true
			})
		}
	}
}

// TestCommandPackagesUseCommandStreams asserts command-domain packages
// under internal/cli render output through the command stream
// (cmd.OutOrStdout / cmd.ErrOrStderr) rather than writing directly to
// os.Stdout or os.Stderr. Direct process-stream writes bypass output
// capture and cannot be redirected by a caller.
func TestCommandPackagesUseCommandStreams(t *testing.T) {
	for dir, paths := range cliGoFiles(t) {
		if dir == "" || commandPackageExemptDirs[dir] {
			continue // boundary packages, not command packages
		}
		for _, path := range paths {
			fset, file := parseGo(t, path)
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok || pkgIdent.Name != "os" {
					return true
				}
				if sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr" {
					pos := fset.Position(sel.Pos())
					t.Errorf("%s:%d: command package internal/cli/%s references os.%s; render through cmd.OutOrStdout()/cmd.ErrOrStderr() instead", path, pos.Line, dir, sel.Sel.Name)
				}
				return true
			})
		}
	}
}

// TestCommandPackagesAvoidStoredContext asserts command-domain packages
// under internal/cli do not store a context.Context in a struct field.
// The operation context flows in via cmd.Context(); parking it on a
// struct detaches cancellation and the deadline from the call site.
func TestCommandPackagesAvoidStoredContext(t *testing.T) {
	for dir, paths := range cliGoFiles(t) {
		if dir == "" || commandPackageExemptDirs[dir] {
			continue
		}
		for _, path := range paths {
			fset, file := parseGo(t, path)
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, field := range st.Fields.List {
					// Limitation: only qualified context.Context field
					// types are matched. A dot-imported, unqualified
					// Context field would not be a SelectorExpr and
					// would slip past — the project does not dot-import
					// the context package, so this is not a real risk.
					sel, ok := field.Type.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					pkgIdent, ok := sel.X.(*ast.Ident)
					if !ok {
						continue
					}
					if pkgIdent.Name == "context" && sel.Sel.Name == "Context" {
						pos := fset.Position(field.Pos())
						t.Errorf("%s:%d: command package internal/cli/%s struct %s stores a context.Context field; pass context per call via cmd.Context()", path, pos.Line, dir, ts.Name.Name)
					}
				}
				return true
			})
		}
	}
}

// isSelectorCall reports whether call is a call of the form pkg.Name(...).
func isSelectorCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == pkg && sel.Sel.Name == name
}
