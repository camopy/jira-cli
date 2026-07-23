// MOTIVATION: output-contract regressions are a recurring upstream
// class — empty lists exiting non-zero, conflicting output modes,
// --debug leaking into stdout. Comments in this file are PROVENANCE
// ONLY and MUST NOT be a source of implementation, fixtures, wording,
// or test logic.
package guardrails

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"gopkg.in/yaml.v3"
)

// Empty lists MUST be a successful exit-0 result with an empty issues
// array, not a failure or null body. We exercise the envelope layer
// directly — the CLI integration test at tests/contract/issue_list_*.go
// covers the end-to-end path.
func TestEmptyListEnvelopeIsExitZeroWithEmptyArray(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "issue.list", Timestamp: "t"},
		Data:     map[string]any{"issues": []any{}, "count": 0},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("envelope not valid JSON: %v\n%s", err, buf.String())
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong type: %v", got["data"])
	}
	issues, _ := data["issues"].([]any)
	if issues == nil {
		t.Fatalf("issues serialized as null; should be empty array")
	}
	if len(issues) != 0 {
		t.Fatalf("issues should be empty, got %d", len(issues))
	}
	// Errors and warnings MUST also be empty arrays, never null.
	if got["errors"] == nil {
		t.Fatalf("errors serialized as null; should be empty array")
	}
	if got["warnings"] == nil {
		t.Fatalf("warnings serialized as null; should be empty array")
	}
}

// The JSON envelope shape MUST stay consistent across all command
// names. The required keys are {meta, data, errors, warnings};
// nothing else is implied.
func TestEnvelopeShapeIsStable(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "x", Timestamp: "t"},
		Data:     map[string]any{"k": "v"},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, has := got[key]; !has {
			t.Fatalf("envelope missing required key %q", key)
		}
	}
}

func TestLintChecksProductionBlankErrorsWithoutDisablingTestLint(t *testing.T) {
	type exclusionRule struct {
		Path    string   `yaml:"path"`
		Linters []string `yaml:"linters"`
		Source  string   `yaml:"source"`
		Text    string   `yaml:"text"`
	}
	var cfg struct {
		Run struct {
			Tests bool `yaml:"tests"`
		} `yaml:"run"`
		Linters struct {
			Settings struct {
				Errcheck struct {
					CheckBlank bool `yaml:"check-blank"`
				} `yaml:"errcheck"`
			} `yaml:"settings"`
			Exclusions struct {
				Rules []exclusionRule `yaml:"rules"`
			} `yaml:"exclusions"`
		} `yaml:"linters"`
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", ".golangci.yml"))
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .golangci.yml: %v", err)
	}
	if !cfg.Run.Tests {
		t.Fatal("golangci-lint run.tests = false, want tests linted")
	}
	if !cfg.Linters.Settings.Errcheck.CheckBlank {
		t.Fatal("errcheck.check-blank = false, want production blank assignments checked")
	}

	const testPath = `_test\.go$`
	var testBlankRule *exclusionRule
	for i := range cfg.Linters.Exclusions.Rules {
		rule := &cfg.Linters.Exclusions.Rules[i]
		if len(rule.Linters) == 1 && rule.Linters[0] == "errcheck" {
			if rule.Path != testPath {
				t.Fatalf("errcheck exclusion path = %q, want only %q", rule.Path, testPath)
			}
			testBlankRule = rule
		}
	}
	if testBlankRule == nil {
		t.Fatal("missing narrow _test.go errcheck blank-assignment exclusion")
	}
	if testBlankRule.Source == "" || testBlankRule.Text == "" {
		t.Fatalf("test errcheck exclusion is not limited by source and finding text: %#v", testBlankRule)
	}
}

func TestCommandPackagesPropagateOutputHelperErrors(t *testing.T) {
	for dir, paths := range cliGoFiles(t) {
		if dir == "" || commandPackageExemptDirs[dir] {
			continue
		}
		for _, path := range paths {
			fset, file := parseGo(t, path)
			for _, call := range discardedOutputHelperCalls(file) {
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d: command package internal/cli/%s discards %s; return or handle its error", path, pos.Line, dir, outputHelperName(call))
			}
			for _, call := range directCommandStreamWrites(file) {
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d: command package internal/cli/%s writes directly to a Cobra stream; use a tracked output helper", path, pos.Line, dir)
			}
		}
	}
}

func TestOutputHelperGuardRejectsDiscardedResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "bare command helper call",
			source: `package command
import "github.com/matcra587/jira-cli/internal/cli/cmdutil"
func bad(cmd *cobra.Command) {
	cmdutil.WriteEnvelope(cmd, "example", nil)
}`,
			want: 1,
		},
		{
			name: "blank direct renderer result",
			source: `package command
import out "github.com/matcra587/jira-cli/internal/cli"
func bad(w io.Writer) {
	_ = out.WriteEnvelope(w, out.Envelope{})
}`,
			want: 1,
		},
		{
			name: "returned helper result",
			source: `package command
import "github.com/matcra587/jira-cli/internal/cli/cmdutil"
func good(cmd *cobra.Command) error {
	return cmdutil.WriteEnvelope(cmd, "example", nil)
}`,
		},
		{
			name: "checked helper result",
			source: `package command
import "github.com/matcra587/jira-cli/internal/cli/cmdutil"
func good(cmd *cobra.Command) error {
	if err := cmdutil.WriteEnvelope(cmd, "example", nil); err != nil {
		return err
	}
	return nil
}`,
		},
		{
			name: "shadowed package name",
			source: `package command
import cli "github.com/matcra587/jira-cli/internal/cli"
type local struct{}
func (local) WriteEnvelope() {}
func good() {
	cli := local{}
	cli.WriteEnvelope()
}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, file := parseGoSource(t, tc.source)
			if got := len(discardedOutputHelperCalls(file)); got != tc.want {
				t.Fatalf("discarded output-helper findings = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOutputHelperGuardRejectsDirectCommandStreamWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{
			name: "fmt print",
			source: `package command
import (
	fmtalias "fmt"
	"github.com/spf13/cobra"
)
func bad(cmd *cobra.Command) {
	_, _ = fmtalias.Fprintln(cmd.OutOrStdout(), "data")
}`,
		},
		{
			name: "io string",
			source: `package command
import (
	stream "io"
	"github.com/spf13/cobra"
)
func bad(cmd *cobra.Command) {
	_, _ = stream.WriteString(cmd.ErrOrStderr(), "diagnostic")
}`,
		},
		{
			name: "writer method",
			source: `package command
import "github.com/spf13/cobra"
func bad(cmd *cobra.Command) {
	_, _ = cmd.OutOrStdout().Write([]byte("data"))
}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, file := parseGoSource(t, tc.source)
			if got := len(directCommandStreamWrites(file)); got != 1 {
				t.Fatalf("direct command-stream findings = %d, want 1", got)
			}
		})
	}

	_, file := parseGoSource(t, `package command
import "fmt"
type local struct{}
func (local) OutOrStdout() io.Writer { return nil }
func allowed(cmd local) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "data")
}`)
	if got := len(directCommandStreamWrites(file)); got != 0 {
		t.Fatalf("direct command-stream findings = %d, want 0 for non-Cobra receiver", got)
	}
}

func directCommandStreamWrites(file *ast.File) []*ast.CallExpr {
	fmtNames := importedPackageNames(file, "fmt")
	ioNames := importedPackageNames(file, "io")
	cobraNames := importedPackageNames(file, "github.com/spf13/cobra")
	cobraDotImported := dotImportsPackage(file, "github.com/spf13/cobra")
	var findings []*ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Write" && isCommandStreamCall(sel.X, cobraNames, cobraDotImported) {
			findings = append(findings, call)
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || len(call.Args) == 0 ||
			!isCommandStreamCall(call.Args[0], cobraNames, cobraDotImported) {
			return true
		}
		if fmtNames[pkg.Name] && strings.HasPrefix(sel.Sel.Name, "Fprint") {
			findings = append(findings, call)
		}
		if ioNames[pkg.Name] && sel.Sel.Name == "WriteString" {
			findings = append(findings, call)
		}
		return true
	})
	return findings
}

func isCommandStreamCall(
	expr ast.Expr,
	cobraNames map[string]bool,
	cobraDotImported bool,
) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok &&
		(sel.Sel.Name == "OutOrStdout" || sel.Sel.Name == "ErrOrStderr") &&
		expressionIsCobraCommand(sel.X, cobraNames, cobraDotImported)
}

func discardedOutputHelperCalls(file *ast.File) []*ast.CallExpr {
	packages := map[string]string{}
	for name := range importedPackageNames(file, "github.com/matcra587/jira-cli/internal/cli") {
		packages[name] = "cli"
	}
	for name := range importedPackageNames(file, "github.com/matcra587/jira-cli/internal/cli/cmdutil") {
		packages[name] = "cmdutil"
	}
	var findings []*ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ExprStmt:
			if call, ok := node.X.(*ast.CallExpr); ok && isOutputHelperCall(call, packages) {
				findings = append(findings, call)
			}
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			blank, ok := node.Lhs[0].(*ast.Ident)
			if !ok || blank.Name != "_" {
				return true
			}
			if call, ok := node.Rhs[0].(*ast.CallExpr); ok && isOutputHelperCall(call, packages) {
				findings = append(findings, call)
			}
		case *ast.GoStmt:
			if isOutputHelperCall(node.Call, packages) {
				findings = append(findings, node.Call)
			}
		case *ast.DeferStmt:
			if isOutputHelperCall(node.Call, packages) {
				findings = append(findings, node.Call)
			}
		}
		return true
	})
	return findings
}

func isOutputHelperCall(call *ast.CallExpr, packages map[string]string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Obj != nil {
		return false
	}
	switch packages[pkg.Name] {
	case "cmdutil":
		return strings.HasPrefix(sel.Sel.Name, "Write") &&
			strings.Contains(sel.Sel.Name, "Envelope")
	case "cli":
		switch sel.Sel.Name {
		case "RouteWarnings",
			"WriteCommandPlain",
			"WriteCompact",
			"WriteEnvelope",
			"WriteEnvelopeDocument",
			"WriteHumanJSON",
			"WriteHumanTOML",
			"WritePlain":
			return true
		}
	}
	return false
}

func outputHelperName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "<output helper>"
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return sel.Sel.Name
	}
	return pkg.Name + "." + sel.Sel.Name
}
