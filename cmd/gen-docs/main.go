// Command gen-docs renders the jira command tree into Markdown reference pages
// for the docs site. It is a developer/CI tool, never compiled into the jira
// binary. It mirrors the GitHub CLI's cmd/gen-docs: build the same Cobra tree
// the binary uses, then walk it with internal/docs.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/matcra587/jira-cli/internal/cli/root"
	"github.com/matcra587/jira-cli/internal/cli/runtime"
	"github.com/matcra587/jira-cli/internal/docs"
)

func main() {
	docPath := flag.String("doc-path", "", "path to write generated reference pages (required)")
	llmsPath := flag.String("llms-path", "", "path to write llms.txt (optional)")
	website := flag.Bool("website", false, "generate site-relative links between pages")
	flag.Parse()
	if *docPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "--doc-path is required")
		os.Exit(1)
	}
	if err := run(*docPath, *llmsPath, *website); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gen-docs: %v\n", err)
		os.Exit(1)
	}
}

func run(docPath, llmsPath string, website bool) error {
	if err := os.MkdirAll(docPath, 0o750); err != nil {
		return err
	}
	rt, err := runtime.New()
	if err != nil {
		return err
	}
	tree := root.New(rt)
	// No effect on internal/docs (which never emits an auto-gen tag); set for parity/defense if the tree is ever walked by cobra's own doc generator.
	tree.DisableAutoGenTag = true

	prepender := func(string) string {
		// MkDocs/Material front-matter; no generation timestamp.
		return "---\nsearch:\n  boost: 0.5\n---\n\n"
	}
	linkHandler := func(name string) string {
		if website {
			return "./" + strings.TrimSuffix(name, ".md") + "/"
		}
		return name
	}
	if err = docs.GenMarkdownTreeCustom(tree, docPath, prepender, linkHandler); err != nil {
		return err
	}
	if llmsPath == "" {
		return nil
	}
	f, err := os.Create(llmsPath)
	if err != nil {
		return err
	}
	if err = docs.GenLLMsTxt(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
