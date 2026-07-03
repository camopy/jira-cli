package adf

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// The matrix computes its capability flags from three sets: authorableNodes
// / authorableMarks (what FromMarkdownLossy emits) and
// renderableMarkdownNodes / renderableMarkdownMarks (what the Markdown
// renderer handles). These tests pin each set to the code's actual
// behavior so a matrix claim can never drift from the implementation.

// authoringCorpus exercises every Markdown construct the converter
// supports, so the emitted node/mark universe equals the declared
// authorable sets.
const authoringCorpus = "# Heading\n\n" +
	"Plain **bold** *italic* ~~struck~~ `code` [link](https://example.com) text.\n\n" +
	"Line one  \nline two after a hard break.\n\n" +
	"- bullet one\n- bullet two\n\n" +
	"1. ordered one\n2. ordered two\n\n" +
	"- [ ] open task\n- [x] done task\n\n" +
	"> quoted paragraph\n\n" +
	"```go\ncode block\n```\n\n" +
	"| H1 | H2 |\n| --- | --- |\n| a | b |\n\n" +
	"---\n"

func collectEmitted(node Node, nodes, marks map[string]bool) {
	if node.Type != "" {
		nodes[node.Type] = true
	}
	for _, m := range node.Marks {
		marks[m.Type] = true
	}
	for _, child := range node.Content {
		collectEmitted(child, nodes, marks)
	}
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func assertSetsEqual(t *testing.T, what string, got, want map[string]bool) {
	t.Helper()
	for name := range got {
		if !want[name] {
			t.Errorf("%s: %q is produced by the code but missing from the declared set", what, name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("%s: %q is declared but the corpus never produced it — stale claim or corpus gap", what, name)
		}
	}
}

// TestAuthorableSetsMatchConverterBehavior converts the corpus and demands
// the emitted node/mark universe equals authorableNodes/authorableMarks
// exactly, in both directions.
func TestAuthorableSetsMatchConverterBehavior(t *testing.T) {
	doc, warnings, err := FromMarkdownLossy(authoringCorpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		if w.Lossy {
			t.Fatalf("the authoring corpus must convert losslessly, got %+v", w)
		}
	}
	nodes := map[string]bool{"doc": true}
	marks := map[string]bool{}
	for _, n := range doc.Content {
		collectEmitted(n, nodes, marks)
	}
	assertSetsEqual(t, "authorable nodes", nodes, authorableNodes)
	assertSetsEqual(t, "authorable marks", marks, authorableMarks)
	t.Logf("corpus emitted %d node types, %d mark types", len(sorted(nodes)), len(sorted(marks)))
}

// TestRenderableSetsMirrorRendererSwitches parses render.go and demands
// the case literals of markdownBlock and markdownText equal the declared
// renderable sets — the source-level contract that replaces the old
// "keep this list in sync by hand" comment in lossy.go.
func TestRenderableSetsMirrorRendererSwitches(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "render.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	nodeCases := switchCaseStrings(t, file, "markdownBlock")
	// The doc root is rendered by ToMarkdown itself, and table structure is
	// walked by markdownTable directly — neither dispatches through
	// markdownBlock's switch.
	for _, structural := range []string{"doc", "tableRow", "tableCell", "tableHeader"} {
		nodeCases[structural] = true
	}
	assertSetsEqual(t, "renderable nodes vs markdownBlock switch", nodeCases, renderableMarkdownNodes)

	markCases := switchCaseStrings(t, file, "markdownText")
	assertSetsEqual(t, "renderable marks vs markdownText switch", markCases, renderableMarkdownMarks)
}

// switchCaseStrings collects every string literal used as a case value
// anywhere inside the named function.
func switchCaseStrings(t *testing.T, file *ast.File, funcName string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					out[lit.Value[1:len(lit.Value)-1]] = true
				}
			}
			return true
		})
		return out
	}
	t.Fatalf("function %s not found in render.go", funcName)
	return nil
}
