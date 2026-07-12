package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/matcra587/jira-cli/internal/cli/root"
	"github.com/matcra587/jira-cli/internal/cli/runtime"
	"github.com/matcra587/jira-cli/internal/docs"
	"github.com/spf13/cobra"
)

func main() {
	docPath := flag.String("doc-path", "", "path to write generated reference pages")
	llmsPath := flag.String("llms-path", "", "path to write llms.txt (optional)")
	navConfig := flag.String("nav-config", "", "path to zensical.toml whose reference-nav block to regenerate (optional)")
	website := flag.Bool("website", false, "generate site-relative links between pages")
	clean := flag.Bool("clean", false, "remove doc-path before generating, so pages for deleted commands don't linger")
	check := flag.Bool("check", false, "with --nav-config, verify the nav is up to date instead of writing it")
	flag.Parse()
	if *docPath == "" && *navConfig == "" {
		_, _ = fmt.Fprintln(os.Stderr, "at least one of --doc-path or --nav-config is required")
		os.Exit(1)
	}
	if err := run(*docPath, *llmsPath, *navConfig, *website, *clean, *check); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gen-docs: %v\n", err)
		os.Exit(1)
	}
}

func run(docPath, llmsPath, navConfig string, website, clean, check bool) error {
	rt, err := runtime.New()
	if err != nil {
		return err
	}
	tree := root.New(rt)
	// No effect on internal/docs (which never emits an auto-gen tag); set for parity/defense if the tree is ever walked by cobra's own doc generator.
	tree.DisableAutoGenTag = true

	if docPath != "" {
		if err = generatePages(tree, docPath, llmsPath, clean); err != nil {
			return err
		}
	}
	if navConfig != "" {
		if err = updateNav(tree, navConfig, check); err != nil {
			return err
		}
	}
	// Cross-page links are emitted as relative .md paths; Zensical (like MkDocs)
	// resolves them to the correct site URLs at build time, so no rewriting is
	// needed. The --website flag is retained for compatibility but no longer
	// changes link output.
	_ = website
	return nil
}

func generatePages(tree *cobra.Command, docPath, llmsPath string, clean bool) error {
	if clean {
		if err := os.RemoveAll(docPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(docPath, 0o750); err != nil {
		return err
	}
	prepender := func(path string) string {
		// Zensical (MkDocs/Material) front-matter. The command path is derived
		// from the file's path relative to the reference root, since the nested
		// layout names section pages index.md: e.g.
		//   jira/agent/index.md   -> "jira agent"
		//   jira/issue/create.md  -> "jira issue create"
		// so each page carries a clean title and description. No generation
		// timestamp, so output stays byte-stable for the CI drift gate.
		rel, err := filepath.Rel(docPath, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimSuffix(rel, "/index.md")
		rel = strings.TrimSuffix(rel, ".md")
		cmd := strings.ReplaceAll(rel, "/", " ")
		return "---\n" +
			"title: " + cmd + "\n" +
			"description: Dynamically generated reference for " + cmd + "\n" +
			"search:\n  boost: 0.5\n" +
			"---\n\n"
	}
	linkHandler := func(name string) string { return name }
	if err := docs.GenMarkdownTreeCustom(tree, docPath, prepender, linkHandler); err != nil {
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

// updateNav regenerates the reference-nav block in the zensical config from the
// command tree. With check set it verifies the block is current and returns an
// error if it has drifted, instead of writing.
func updateNav(tree *cobra.Command, navConfig string, check bool) error {
	raw, err := os.ReadFile(navConfig)
	if err != nil {
		return err
	}
	navBlock := docs.GenReferenceNav(tree, "    ")
	updated, err := docs.SpliceReferenceNav(string(raw), navBlock)
	if err != nil {
		return err
	}
	if check {
		if string(raw) != updated {
			return fmt.Errorf("reference nav in %s is out of date; run `mise run docs:gen` to regenerate", navConfig)
		}
		return nil
	}
	if updated == string(raw) {
		return nil
	}
	return os.WriteFile(navConfig, []byte(updated), 0o644) //nolint:gosec // config file, not a secret
}
