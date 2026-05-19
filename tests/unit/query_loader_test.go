package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestNamedQueryLoaderFrontmatterAndRawJQL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine.jql"), []byte("---\nname: Mine\nproject: PROJ\n---\nassignee = currentUser()"), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := config.LoadQueries(dir)
	if err != nil {
		t.Fatalf("LoadQueries() error = %v", err)
	}
	if queries["mine"].Name != "Mine" || queries["mine"].JQL != "assignee = currentUser()" {
		t.Fatalf("query = %+v", queries["mine"])
	}
}

func TestNamedQueryLoaderExpandsEnvInDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "queries")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mine.jql"), []byte("assignee = currentUser()"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JIRA_CLI_QUERIES_ROOT", root)

	queries, err := config.LoadQueries("$JIRA_CLI_QUERIES_ROOT/queries")
	if err != nil {
		t.Fatalf("LoadQueries() error = %v", err)
	}
	if queries["mine"].JQL != "assignee = currentUser()" {
		t.Fatalf("query = %+v", queries["mine"])
	}
}

func TestNamedQueryLoaderParsesQuotedYAMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	body := `---
name: "Mine: Open Bugs"
description: "Active bugs: do not close #incident"
project: "PROJ"
---
summary ~ "--- not frontmatter" AND status = Open
`
	if err := os.WriteFile(filepath.Join(dir, "bugs.jql"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := config.LoadQueries(dir)
	if err != nil {
		t.Fatalf("LoadQueries() error = %v", err)
	}
	got := queries["bugs"]
	if got.Name != "Mine: Open Bugs" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Description != "Active bugs: do not close #incident" {
		t.Fatalf("Description = %q", got.Description)
	}
	if got.Project != "PROJ" {
		t.Fatalf("Project = %q", got.Project)
	}
	if got.JQL != `summary ~ "--- not frontmatter" AND status = Open` {
		t.Fatalf("JQL = %q", got.JQL)
	}
}

func TestNamedQueryLoaderParsesTOMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	body := `+++
name = "Release: Blockers"
description = "Has colon: and #literal"
project = "REL"
+++
project = REL AND priority = Highest
`
	if err := os.WriteFile(filepath.Join(dir, "release.jql"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := config.LoadQueries(dir)
	if err != nil {
		t.Fatalf("LoadQueries() error = %v", err)
	}
	got := queries["release"]
	if got.Name != "Release: Blockers" || got.Description != "Has colon: and #literal" || got.Project != "REL" {
		t.Fatalf("query metadata = %+v", got)
	}
	if got.JQL != "project = REL AND priority = Highest" {
		t.Fatalf("JQL = %q", got.JQL)
	}
}

func TestNamedQueryLoaderRejectsMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: [unterminated\n---\nproject = PROJ\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.jql"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadQueries(dir); err == nil {
		t.Fatal("LoadQueries() error = nil for malformed frontmatter")
	}
}
