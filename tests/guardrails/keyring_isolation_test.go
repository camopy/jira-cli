// MOTIVATION: the contract and integration suites exec the real jira binary,
// which stores credentials in the OS keyring. Each suite's TestMain sets
// JIRA_KEYRING_SERVICE to a throwaway namespace so a credential-mutating
// command can never reach a developer's real "jira-cli" credential — but that
// only holds if every exec inherits the parent process environment. A test that
// assigns a command's Env a from-scratch composite literal silently drops the
// override and reopens the footgun (a real credential was once deleted on every
// `go test` run). These guards fail on such an assignment, and fail if a suite
// stops setting the override at all, so the isolation cannot be quietly removed.
// Comments in this file are PROVENANCE ONLY and MUST NOT be a source of
// implementation, fixtures, wording, or test logic.
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

// keyringIsolatedSuites are the test suites that drive the real binary and so
// MUST confine its keyring use to a throwaway namespace via an inherited
// JIRA_KEYRING_SERVICE. The live suite is intentionally absent: it
// authenticates against a real tenant with the developer's real credential and
// must reach the real keyring.
var keyringIsolatedSuites = []string{"../contract", "../integration"}

// keyringIsolationEnv is the environment variable each isolated suite sets to
// redirect credential storage away from the real "jira-cli" service.
const keyringIsolationEnv = "JIRA_KEYRING_SERVICE"

// suiteGoFiles returns every .go file directly under dir.
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

// TestIsolatedSuitesSetKeyringNamespace fails if an isolated suite no longer
// references JIRA_KEYRING_SERVICE — i.e. the keyring isolation was removed.
func TestIsolatedSuitesSetKeyringNamespace(t *testing.T) {
	for _, dir := range keyringIsolatedSuites {
		found := false
		for _, path := range suiteGoFiles(t, dir) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(b), keyringIsolationEnv) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("suite %s no longer sets %s; the real OS keyring is no longer isolated from this suite", dir, keyringIsolationEnv)
		}
	}
}

// TestIsolatedSuitesInheritProcessEnv fails when a test in an isolated suite
// assigns a command's Env to a from-scratch composite literal (e.g.
// `cmd.Env = []string{...}`). That drops the inherited keyringIsolationEnv and
// lets the exec'd binary mutate the real keyring. Tests must extend the parent
// environment (os.Environ() / cmd.Environ()) instead.
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
					if !ok || sel.Sel.Name != "Env" {
						continue
					}
					if i >= len(assign.Rhs) {
						continue
					}
					if _, ok := assign.Rhs[i].(*ast.CompositeLit); ok {
						pos := fset.Position(assign.Pos())
						t.Errorf("%s:%d: assigns .Env from a composite literal, dropping the inherited %s; base the env on os.Environ()/cmd.Environ() instead", path, pos.Line, keyringIsolationEnv)
					}
				}
				return true
			})
		}
	}
}
