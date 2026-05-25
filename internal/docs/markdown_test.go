package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func sampleTree() *cobra.Command {
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal"}
	root.PersistentFlags().String("output", "auto", "output mode")
	issue := &cobra.Command{Use: "issue", Short: "Work with issues"}
	create := &cobra.Command{
		Use:     "create",
		Short:   "Create an issue",
		Long:    "Create an issue in a project.",
		Example: "  jira issue create --project KAN --summary x",
		Run:     func(*cobra.Command, []string) {},
	}
	create.Flags().String("project", "", "project key")
	issue.AddCommand(create)
	root.AddCommand(issue)
	return root
}

func TestGenMarkdownCustomIsDeterministicAndComplete(t *testing.T) {
	cmd, _, err := sampleTree().Find([]string{"issue", "create"})
	if err != nil || cmd.Name() != "create" {
		t.Fatal("could not find issue create in sample tree")
	}
	var a, b bytes.Buffer
	prep := func(string) string { return "---\ntitle: x\n---\n\n" }
	link := func(s string) string { return s }
	if err = GenMarkdownCustom(cmd, &a, prep, link); err != nil {
		t.Fatalf("GenMarkdownCustom: %v", err)
	}
	if err = GenMarkdownCustom(cmd, &b, prep, link); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if a.String() != b.String() {
		t.Fatal("output is not byte-stable across runs")
	}
	out := a.String()
	for _, want := range []string{"# jira issue create", "Create an issue in a project.", "jira issue create --project", "--project", "### Options", "### SEE ALSO", "jira issue"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "auto generated") {
		t.Errorf("output must not embed a generation marker:\n%s", out)
	}
	if regexp.MustCompile(`\b20\d\d\b`).MatchString(out) {
		t.Errorf("output must not embed a generation timestamp (4-digit year):\n%s", out)
	}
}

func TestGenMarkdownTreeWritesOnePagePerCommand(t *testing.T) {
	dir := t.TempDir()
	prep := func(string) string { return "" }
	link := func(s string) string { return s }
	if err := GenMarkdownTreeCustom(sampleTree(), dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}
	for _, name := range []string{"jira.md", "jira_issue.md", "jira_issue_create.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
}

// TestGenMarkdownTreeExcludesHiddenAndHelpTopics asserts that hidden commands
// and additional-help-topic commands produce no page file and do not appear in
// a parent command's Subcommands / SEE ALSO output.
func TestGenMarkdownTreeExcludesHiddenAndHelpTopics(t *testing.T) {
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal"}
	visible := &cobra.Command{
		Use:   "visible",
		Short: "A visible subcommand",
		Run:   func(*cobra.Command, []string) {},
	}
	hidden := &cobra.Command{
		Use:    "hidden",
		Short:  "A hidden subcommand",
		Hidden: true,
		Run:    func(*cobra.Command, []string) {},
	}
	// An additional-help-topic command: no Run and no subcommands.
	helpTopic := &cobra.Command{
		Use:   "help-topic",
		Short: "A help topic",
	}
	root.AddCommand(visible, hidden, helpTopic)

	dir := t.TempDir()
	prep := func(string) string { return "" }
	link := func(s string) string { return s }
	if err := GenMarkdownTreeCustom(root, dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}

	// Neither excluded command should produce a page file.
	for _, name := range []string{"jira_hidden.md", "jira_help-topic.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("unexpected page file written for excluded command: %s", name)
		}
	}

	// The visible command's page must exist.
	if _, err := os.Stat(filepath.Join(dir, "jira_visible.md")); err != nil {
		t.Errorf("expected jira_visible.md: %v", err)
	}

	// The root page must not mention the hidden or help-topic commands.
	rootPage, err := os.ReadFile(filepath.Join(dir, "jira.md"))
	if err != nil {
		t.Fatalf("reading jira.md: %v", err)
	}
	for _, excluded := range []string{"hidden", "help-topic"} {
		if strings.Contains(string(rootPage), excluded) {
			t.Errorf("root page must not reference excluded command %q:\n%s", excluded, rootPage)
		}
	}
}

// TestGenMarkdownTreeLinkHandlerApplied asserts that a non-identity linkHandler
// is applied to subcommand links and SEE ALSO entries in rendered output.
func TestGenMarkdownTreeLinkHandlerApplied(t *testing.T) {
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal"}
	sub := &cobra.Command{
		Use:   "issue",
		Short: "Work with issues",
		Run:   func(*cobra.Command, []string) {},
	}
	root.AddCommand(sub)

	dir := t.TempDir()
	prep := func(string) string { return "" }
	// Website-style linkHandler: "x.md" -> "./x/"
	link := func(name string) string {
		return "./" + strings.TrimSuffix(name, ".md") + "/"
	}
	if err := GenMarkdownTreeCustom(root, dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}

	rootPage, err := os.ReadFile(filepath.Join(dir, "jira.md"))
	if err != nil {
		t.Fatalf("reading jira.md: %v", err)
	}
	want := "./jira_issue/"
	if !strings.Contains(string(rootPage), want) {
		t.Errorf("root page does not contain rewritten link %q:\n%s", want, rootPage)
	}
}

// TestGenMarkdownTreeBasenameAnnotation asserts that a command with the
// markdown:basename annotation writes its page under the overridden name.
func TestGenMarkdownTreeBasenameAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal"}
	sub := &cobra.Command{
		Use:         "issue",
		Short:       "Work with issues",
		Annotations: map[string]string{basenameAnnotation: "custom_name"},
		Run:         func(*cobra.Command, []string) {},
	}
	root.AddCommand(sub)

	dir := t.TempDir()
	prep := func(string) string { return "" }
	link := func(s string) string { return s }
	if err := GenMarkdownTreeCustom(root, dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}

	// The overridden name must be used.
	if _, err := os.Stat(filepath.Join(dir, "custom_name.md")); err != nil {
		t.Errorf("expected custom_name.md from basename annotation: %v", err)
	}
	// The default name must NOT exist.
	if _, err := os.Stat(filepath.Join(dir, "jira_issue.md")); err == nil {
		t.Error("default basename jira_issue.md must not be written when annotation overrides it")
	}
}

// TestGenMarkdownTreeBasenameAnnotationCannotEscapeDir asserts that a
// directory-bearing basename annotation is reduced to its final path element,
// so a generated page can never be written outside the output directory.
func TestGenMarkdownTreeBasenameAnnotationCannotEscapeDir(t *testing.T) {
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal"}
	sub := &cobra.Command{
		Use:         "issue",
		Short:       "Work with issues",
		Annotations: map[string]string{basenameAnnotation: "../escape"},
		Run:         func(*cobra.Command, []string) {},
	}
	root.AddCommand(sub)

	dir := t.TempDir()
	prep := func(string) string { return "" }
	link := func(s string) string { return s }
	if err := GenMarkdownTreeCustom(root, dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}

	// The page must land inside dir under the sanitized name.
	if _, err := os.Stat(filepath.Join(dir, "escape.md")); err != nil {
		t.Errorf("expected sanitized escape.md inside the output dir: %v", err)
	}
	// Nothing must be written to the parent directory.
	if _, err := os.Stat(filepath.Join(dir, "..", "escape.md")); err == nil {
		t.Error("basename annotation escaped the output directory")
	}
}
