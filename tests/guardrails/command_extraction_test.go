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
	pathpkg "path"
	"path/filepath"
	"strconv"
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
	// cmdutil is the shared command-helper layer (envelope writers,
	// client/profile accessors, output-mode and gate helpers), not a
	// command-domain package — it is a dependency boundary like runtime.
	"cmdutil": true,
}

// commandStreamExemptDirs are command-domain packages that legitimately
// reference os.Stdout for process-level TTY/env detection (not
// command-stream output) and so are exempt from the command-stream check
// ONLY. They remain bound by the os.Exit and stored-context guards.
var commandStreamExemptDirs = map[string]bool{
	// root is the CLI assembly layer (Execute, New, ExitCode). Its only
	// os.Stdout references are cli.Detect(os.Stdout) in detectOutput and
	// jsonEnvelopeRequested — inspecting the real process descriptor to
	// resolve output mode before cobra dispatch. It writes nothing to
	// os.Stdout, stores no context.Context, and never calls os.Exit.
	"root": true,
	// tui owns the real terminal. Its process-stream reference is a
	// RequireTTY descriptor check before the dashboard takes over, not
	// command output.
	"tui": true,
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

func parseGoSource(t *testing.T, source string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "guard_fixture.go", source, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse guard fixture: %v", err)
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
		if dir == "" || commandPackageExemptDirs[dir] || commandStreamExemptDirs[dir] {
			continue // boundary packages and stream-only-exempt command packages
		}
		for _, path := range paths {
			fset, file := parseGo(t, path)
			for _, sel := range processStreamReferences(file) {
				pos := fset.Position(sel.Pos())
				t.Errorf("%s:%d: command package internal/cli/%s references os.%s; render through cmd.OutOrStdout()/cmd.ErrOrStderr() instead", path, pos.Line, dir, sel.Sel.Name)
			}
		}
	}
}

func TestCommandStreamGuardRejectsDirectProcessWrites(t *testing.T) {
	_, file := parseGoSource(t, `package command

import (
	"fmt"
	stdos "os"
)

func bad() {
	fmt.Fprintln(stdos.Stdout, "data")
	stdos.Stderr.Write([]byte("diagnostic"))
}
`)
	if got := len(processStreamReferences(file)); got != 2 {
		t.Fatalf("process-stream findings = %d, want 2", got)
	}
}

func processStreamReferences(file *ast.File) []*ast.SelectorExpr {
	osNames := importedPackageNames(file, "os")
	var findings []*ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Obj != nil || !osNames[pkgIdent.Name] {
			return true
		}
		if sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr" {
			findings = append(findings, sel)
		}
		return true
	})
	return findings
}

// TestCommandPackagesUseRunE keeps every authored cobra handler on the
// error-returning seam. A Run callback cannot propagate a renderer or command
// failure to root.
func TestCommandPackagesUseRunE(t *testing.T) {
	for dir, paths := range cliGoFiles(t) {
		if dir == "" || commandPackageExemptDirs[dir] {
			continue
		}
		for _, path := range paths {
			fset, file := parseGo(t, path)
			for _, handler := range runHandlers(file) {
				pos := fset.Position(handler.Pos())
				t.Errorf("%s:%d: command package internal/cli/%s defines Run; use RunE so command and output failures propagate", path, pos.Line, dir)
			}
		}
	}
}

func TestRunEGuardRejectsRunHandler(t *testing.T) {
	_, file := parseGoSource(t, `package command

import cobraalias "github.com/spf13/cobra"

func newCommand() any {
	cmd := &cobraalias.Command{
		Run: func(cmd *cobraalias.Command, _ []string) {
			cmdutil.WriteEnvelope(cmd, "example", nil)
		},
	}
	cmd.Run = func(cmd *cobraalias.Command, _ []string) {
		cmdutil.WriteEnvelope(cmd, "example", nil)
	}
	other := &cobraalias.Command{Run: runHandler}
	other.Run = runHandler
	return cmd
}

func runHandler(*cobraalias.Command, []string) {}
`)
	if got := len(runHandlers(file)); got != 4 {
		t.Fatalf("Run handler findings = %d, want 4", got)
	}
}

func TestRunEGuardAllowsUnrelatedRunFields(t *testing.T) {
	_, file := parseGoSource(t, `package command

type options struct {
	Run bool
}

func configure(worker *options) {
	_ = options{Run: true}
	worker.Run = false
}
`)
	if got := len(runHandlers(file)); got != 0 {
		t.Fatalf("Run handler findings = %d, want 0 for non-Cobra fields", got)
	}
}

func TestRunEGuardRejectsCobraAssignmentsAcrossDeclarationShapes(t *testing.T) {
	_, file := parseGoSource(t, `package command

import cobraalias "github.com/spf13/cobra"

type holder struct {
	cmd *cobraalias.Command
}

func factory() *cobraalias.Command { return &cobraalias.Command{} }
func handler(*cobraalias.Command, []string) {}

func configure(h holder) {
	var declared = &cobraalias.Command{}
	declared.Run = handler
	fromFactory := factory()
	fromFactory.Run = handler
	value := cobraalias.Command{}
	value.Run = handler
	h.cmd.Run = handler
	fromFactory.Run = external.Handler
}
`)
	if got := len(runHandlers(file)); got != 5 {
		t.Fatalf("Run handler findings = %d, want 5 across Cobra declaration shapes", got)
	}
}

func TestRunEGuardRejectsImplicitNestedCobraLiterals(t *testing.T) {
	_, file := parseGoSource(t, `package command

import cobraalias "github.com/spf13/cobra"

func commands() {
	_ = []*cobraalias.Command{{Run: handler}}
	_ = map[string]*cobraalias.Command{"one": {Run: handler}}
}

func handler(*cobraalias.Command, []string) {}
`)
	if got := len(runHandlers(file)); got != 2 {
		t.Fatalf("Run handler findings = %d, want 2 for implicit Cobra literals", got)
	}
}

func TestRunEGuardRejectsCobraAliasesAndEmbedding(t *testing.T) {
	_, file := parseGoSource(t, `package command

import cobraalias "github.com/spf13/cobra"

type commandAlias = cobraalias.Command
type embeddedCommand struct {
	cobraalias.Command
}

func configure() {
	alias := commandAlias{Run: handler}
	alias.Run = handler
	embedded := embeddedCommand{Command: cobraalias.Command{Run: handler}}
	embedded.Run = handler
}

func handler(*cobraalias.Command, []string) {}
`)
	if got := len(runHandlers(file)); got != 4 {
		t.Fatalf("Run handler findings = %d, want 4 for Cobra aliases and embedding", got)
	}
}

func runHandlers(file *ast.File) []ast.Node {
	cobraNames := importedPackageNames(file, "github.com/spf13/cobra")
	cobraDotImported := dotImportsPackage(file, "github.com/spf13/cobra")
	var findings []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if isCobraCommandType(node.Type, cobraNames, cobraDotImported) {
				findings = append(findings, cobraRunFields(node)...)
			}
			if cobraCompositeElementType(node.Type, cobraNames, cobraDotImported) {
				findings = append(findings, implicitCobraRunFields(node)...)
			}
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			sel, ok := node.Lhs[0].(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Run" {
				return true
			}
			if receiverIsProvablyNonCobra(sel.X, cobraNames, cobraDotImported) {
				return true
			}
			findings = append(findings, node)
		}
		return true
	})
	return findings
}

func cobraRunFields(lit *ast.CompositeLit) []ast.Node {
	var findings []ast.Node
	for _, elt := range lit.Elts {
		field, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if ok && key.Name == "Run" {
			findings = append(findings, field)
		}
	}
	return findings
}

func implicitCobraRunFields(outer *ast.CompositeLit) []ast.Node {
	var findings []ast.Node
	for _, elt := range outer.Elts {
		if keyed, ok := elt.(*ast.KeyValueExpr); ok {
			elt = keyed.Value
		}
		if lit, ok := elt.(*ast.CompositeLit); ok && lit.Type == nil {
			findings = append(findings, cobraRunFields(lit)...)
		}
	}
	return findings
}

func isPointerToCobraCommand(expr ast.Expr, cobraNames map[string]bool, cobraDotImported bool) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && isCobraCommandType(star.X, cobraNames, cobraDotImported)
}

func isCobraCommandType(expr ast.Expr, cobraNames map[string]bool, cobraDotImported bool) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		if cobraDotImported && ident.Obj == nil && ident.Name == "Command" {
			return true
		}
		if ident.Obj != nil {
			if spec, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
				return isCobraCommandType(spec.Type, cobraNames, cobraDotImported)
			}
		}
		return false
	}
	if structType, ok := expr.(*ast.StructType); ok {
		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 &&
				(isCobraCommandType(field.Type, cobraNames, cobraDotImported) ||
					isPointerToCobraCommand(field.Type, cobraNames, cobraDotImported)) {
				return true
			}
		}
		return false
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Command" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Obj == nil && cobraNames[pkg.Name]
}

func cobraCompositeElementType(expr ast.Expr, cobraNames map[string]bool, cobraDotImported bool) bool {
	switch container := expr.(type) {
	case *ast.ArrayType:
		return isPointerToCobraCommand(container.Elt, cobraNames, cobraDotImported) ||
			isCobraCommandType(container.Elt, cobraNames, cobraDotImported)
	case *ast.MapType:
		return isPointerToCobraCommand(container.Value, cobraNames, cobraDotImported) ||
			isCobraCommandType(container.Value, cobraNames, cobraDotImported)
	}
	return false
}

func receiverIsProvablyNonCobra(
	expr ast.Expr,
	cobraNames map[string]bool,
	cobraDotImported bool,
) bool {
	receiver, ok := expr.(*ast.Ident)
	if !ok || receiver.Obj == nil {
		return false
	}
	switch decl := receiver.Obj.Decl.(type) {
	case *ast.Field:
		return isKnownNonCobraType(decl.Type, cobraNames, cobraDotImported)
	case *ast.ValueSpec:
		if decl.Type != nil {
			return isKnownNonCobraType(decl.Type, cobraNames, cobraDotImported)
		}
		for i, ident := range decl.Names {
			if ident.Obj == receiver.Obj && i < len(decl.Values) {
				return isKnownNonCobraValue(decl.Values[i], cobraNames, cobraDotImported)
			}
		}
	case *ast.AssignStmt:
		for i, lhs := range decl.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if ok && ident.Obj == receiver.Obj && i < len(decl.Rhs) {
				return isKnownNonCobraValue(decl.Rhs[i], cobraNames, cobraDotImported)
			}
		}
	}
	return false
}

func isKnownNonCobraType(expr ast.Expr, cobraNames map[string]bool, cobraDotImported bool) bool {
	if isCobraCommandType(expr, cobraNames, cobraDotImported) ||
		isPointerToCobraCommand(expr, cobraNames, cobraDotImported) {
		return false
	}
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return isKnownNonCobraType(typed.X, cobraNames, cobraDotImported)
	case *ast.Ident:
		if typed.Obj == nil {
			return true
		}
		spec, ok := typed.Obj.Decl.(*ast.TypeSpec)
		return ok && isKnownNonCobraType(spec.Type, cobraNames, cobraDotImported)
	case *ast.SelectorExpr, *ast.StructType:
		return true
	}
	return false
}

func isKnownNonCobraValue(expr ast.Expr, cobraNames map[string]bool, cobraDotImported bool) bool {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}
	lit, ok := expr.(*ast.CompositeLit)
	return ok && isKnownNonCobraType(lit.Type, cobraNames, cobraDotImported)
}

func importedPackageNames(file *ast.File, importPath string) map[string]bool {
	names := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		name := pathpkg.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "." && name != "_" {
			names[name] = true
		}
	}
	return names
}

func dotImportsPackage(file *ast.File, importPath string) bool {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil && path == importPath && spec.Name != nil && spec.Name.Name == "." {
			return true
		}
	}
	return false
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
