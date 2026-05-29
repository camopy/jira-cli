// MOTIVATION: the contract and integration suites exec the real jira binary.
// Each suite's TestMain sets JIRA_TEST_CREDENTIAL_STORE_DIR so keyring-backed
// fixture profiles use a temp file store instead of the developer's OS
// keyring. A test that assigns a command's Env a from-scratch slice silently
// drops the override and reopens the footgun (a real credential was once
// deleted on every `go test` run). These guards fail on such an assignment, and
// fail if a suite stops setting the file-store override, so the isolation
// cannot be quietly removed.
//
// Known limits (deterrents, not proofs): the env guard catches direct
// `.Env = []string{...}` and `.Env = append([]string{...}, ...)`, but not a
// value laundered through an intermediate variable; and isolation relies on the
// package-level TestMain running, so a `go test <single_file>.go` invocation
// that omits the suite's main_test.go would skip it. Normal package-level
// `go test ./...` — the workflow that caused the incident — is covered.
//
// Comments in this file are PROVENANCE ONLY and MUST NOT be a source of
// implementation, fixtures, wording, or test logic.
package guardrails

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// keyringIsolatedSuites are the test suites that drive the real binary and so
// MUST redirect keyring-backed fixtures to a temp file store via an inherited
// JIRA_TEST_CREDENTIAL_STORE_DIR. The live suite is intentionally absent: it
// authenticates against a real tenant with the developer's real credential and
// must reach the real keyring.
var keyringIsolatedSuites = []string{"../contract", "../integration"}

// keyringIsolationEnv is the environment variable each isolated suite sets to
// redirect credential storage away from the real keyring.
const (
	keyringIsolationEnv = "JIRA_TEST_CREDENTIAL_STORE_DIR"
)

// suiteGoFiles returns every .go file under dir.
func suiteGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

// stringLitValue returns the unquoted value of a string-literal expression, or
// "" with ok=false when expr is not a plain string literal.
func stringLitValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

// TestIsolatedSuitesSetCredentialStoreDir fails unless each isolated suite
// calls Setenv(JIRA_TEST_CREDENTIAL_STORE_DIR, ...). A textual mention is not
// enough — the override must actually be installed before child binaries run.
func TestIsolatedSuitesSetCredentialStoreDir(t *testing.T) {
	for _, dir := range keyringIsolatedSuites {
		isolated := false
		for _, path := range suiteGoFiles(t, dir) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Setenv" || len(call.Args) != 2 {
					return true
				}
				if key, ok := stringLitValue(call.Args[0]); !ok || key != keyringIsolationEnv {
					return true
				}
				if _, ok := call.Args[1].(*ast.Ident); ok {
					isolated = true
					return true
				}
				if val, ok := stringLitValue(call.Args[1]); ok && strings.TrimSpace(val) != "" {
					isolated = true
				}
				return true
			})
		}
		if !isolated {
			t.Errorf("suite %s does not Setenv %s; the real OS keyring is not isolated from this suite", dir, keyringIsolationEnv)
		}
	}
}

// TestIsolatedSuitesInheritProcessEnv fails when a test in an isolated suite
// assigns a command's Env to a from-scratch slice — `.Env = []string{...}` or
// `.Env = append([]string{...}, ...)` — which drops the inherited
// keyringIsolationEnv and lets the exec'd binary mutate the real keyring.
// Tests must extend the parent environment (os.Environ() / cmd.Environ())
// instead.
func TestIsolatedSuitesInheritProcessEnv(t *testing.T) {
	for _, dir := range keyringIsolatedSuites {
		for _, path := range suiteGoFiles(t, dir) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range assign.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Env" || i >= len(assign.Rhs) {
						continue
					}
					if fromScratchEnv(assign.Rhs[i]) {
						pos := fset.Position(assign.Pos())
						t.Errorf("%s:%d: assigns .Env from a from-scratch slice, dropping the inherited %s; base the env on os.Environ()/cmd.Environ() instead", path, pos.Line, keyringIsolationEnv)
					}
				}
				return true
			})
		}
	}
}

// fromScratchEnv reports whether expr builds a slice from scratch rather than
// extending the inherited environment: a `[]string{...}` composite literal, or
// `append([]string{...}, ...)` whose base is such a literal.
func fromScratchEnv(expr ast.Expr) bool {
	if _, ok := expr.(*ast.CompositeLit); ok {
		return true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if ident, ok := call.Fun.(*ast.Ident); !ok || ident.Name != "append" || len(call.Args) == 0 {
		return false
	}
	_, baseIsLiteral := call.Args[0].(*ast.CompositeLit)
	return baseIsLiteral
}
