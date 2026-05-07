// MOTIVATION: customfield handling that branches on field-type names
// in command code is a recurring class of "works for one Jira
// instance, breaks on another" bug. Comments in this file are
// PROVENANCE ONLY and MUST NOT be a source of implementation,
// fixtures, wording, or test logic.
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

// Command code MUST NOT special-case field types via switch/if
// branches on string literals like "cascadingselect" or "user". All
// field-type behavior goes through pkg/jira/customfield.
//
// The guard is intentionally narrow — it parses each .go file under
// cmd/ for switch/case clauses whose case literal matches a registered
// field-type name. Comments and string-format calls are tolerated.
func TestCommandsDoNotSpecialCaseFieldTypes(t *testing.T) {
	registeredTypes := []string{
		"cascadingselect", "select", "multiselect",
		"user", "group", "components", "parent",
		"labels", "version", "fixversions",
	}

	root := filepath.Join("..", "..", "cmd")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range cc.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value := strings.Trim(lit.Value, `"`)
					for _, name := range registeredTypes {
						if value == name {
							pos := fset.Position(lit.Pos())
							t.Errorf("%s:%d switches on field-type literal %q — route through pkg/jira/customfield instead", path, pos.Line, value)
						}
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
