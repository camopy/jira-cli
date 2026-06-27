package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	clib "github.com/gechr/clib/cli/cobra"
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
		Example: "  jira issue create --project JCT --summary x",
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
	for _, want := range []string{"# `jira issue create`", "- **Usage**:", "Create an issue in a project.", "jira issue create --project", "## Options", "### `--project <PROJECT>`"} {
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

// clibSampleTree builds a root with a grouped, enum-bearing persistent flag and
// a `create` subcommand whose flags carry clib metadata (groups, authored
// placeholders, an enum, a repeatable slice, and an angle-bracketed usage).
func clibSampleTree() *cobra.Command {
	// The real jira root is runnable (bare `jira` has behavior), so its own
	// flags render on its page; mirror that here.
	root := &cobra.Command{Use: "jira", Short: "Jira from the terminal", Run: func(*cobra.Command, []string) {}}
	root.PersistentFlags().String("output", "auto", "Output mode")
	clib.Extend(root.PersistentFlags().Lookup("output"), clib.FlagExtra{
		Group:       "Output",
		Enum:        []string{"auto", "json"},
		EnumTerse:   []string{"detect terminal", "JSON envelope"},
		EnumDefault: "auto",
	})

	create := &cobra.Command{Use: "create", Short: "Create an issue", Run: func(*cobra.Command, []string) {}}
	create.Flags().String("project", "", "Project key, e.g. <KEY>")
	create.Flags().StringSlice("label", nil, "Label to attach")
	create.Flags().String("format", "table", "Output format")
	create.Flags().Bool("dry-run", false, "Preview only")
	clib.Extend(create.Flags().Lookup("project"), clib.FlagExtra{Group: "Fields", Placeholder: "KEY"})
	clib.Extend(create.Flags().Lookup("label"), clib.FlagExtra{Group: "Fields"})
	clib.Extend(create.Flags().Lookup("format"), clib.FlagExtra{
		Group:       "Output",
		Placeholder: "FORMAT",
		Enum:        []string{"table", "json"},
		EnumTerse:   []string{"aligned table", "JSON"},
		EnumDefault: "table",
	})
	clib.Extend(create.Flags().Lookup("dry-run"), clib.FlagExtra{Group: "Safety"})
	root.AddCommand(create)
	return root
}

// TestGenMarkdownCustomRendersClibMetadata asserts the generator surfaces the
// clib flag metadata a command carries — grouped sections in task-flow order,
// authored placeholders, repeatable markers, allowed-value lists, and
// angle-bracket-safe descriptions — for the command's own (local) flags.
func TestGenMarkdownCustomRendersClibMetadata(t *testing.T) {
	cmd, _, err := clibSampleTree().Find([]string{"create"})
	if err != nil {
		t.Fatalf("find create: %v", err)
	}
	var b bytes.Buffer
	if err = GenMarkdownCustom(cmd, &b, func(string) string { return "" }, func(s string) string { return s }); err != nil {
		t.Fatalf("GenMarkdownCustom: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"## Fields",
		"### `--project <KEY>`",  // authored placeholder beats the upper-cased name
		"### `--label <LABEL>…`", // repeatable slice gets the ellipsis
		"## Output",
		"### `--format <FORMAT>`",
		"Allowed values:",
		"- `table` — aligned table (default)", // enum value + terse + default marker
		"## Safety",
		"### `--dry-run`",               // bool flag carries no placeholder
		"Project key, e.g. &lt;KEY&gt;", // bare angle brackets escaped, not eaten as a tag
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}

	// Fields (task-flow rank 2) must precede Safety (rank 12).
	if i, j := strings.Index(out, "## Fields"), strings.Index(out, "## Safety"); i < 0 || j < 0 || i > j {
		t.Errorf("Fields should precede Safety; got Fields@%d Safety@%d", i, j)
	}
	// The inherited --output flag belongs to root, not the subcommand: clib hides
	// inherited flags on subcommands, so neither it nor a Global section appears.
	for _, absent := range []string{"--output", "## Global"} {
		if strings.Contains(out, absent) {
			t.Errorf("subcommand page must not show inherited flag %q\n---\n%s", absent, out)
		}
	}
}

// TestGenMarkdownCustomShowsGroupedFlagsOnRoot asserts that a command's own
// (root-level) grouped flags — including persistent ones — render on its page.
func TestGenMarkdownCustomShowsGroupedFlagsOnRoot(t *testing.T) {
	root := clibSampleTree()
	var b bytes.Buffer
	if err := GenMarkdownCustom(root, &b, func(string) string { return "" }, func(s string) string { return s }); err != nil {
		t.Fatalf("GenMarkdownCustom: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"## Output",
		"### `--output <OUTPUT>`",
		"- `auto` — detect terminal (default)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("root page missing %q\n---\n%s", want, out)
		}
	}
}

func TestGenMarkdownTreeWritesOnePagePerCommand(t *testing.T) {
	dir := t.TempDir()
	prep := func(string) string { return "" }
	link := func(s string) string { return s }
	if err := GenMarkdownTreeCustom(sampleTree(), dir, prep, link); err != nil {
		t.Fatalf("GenMarkdownTreeCustom: %v", err)
	}
	// Nested layout: parents become <path>/index.md, leaves <path>.md.
	for _, name := range []string{"jira/index.md", "jira/issue/index.md", "jira/issue/create.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
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
	for _, name := range []string{"jira/hidden.md", "jira/help-topic.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			t.Errorf("unexpected page file written for excluded command: %s", name)
		}
	}

	// The visible command's page must exist.
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash("jira/visible.md"))); err != nil {
		t.Errorf("expected jira/visible.md: %v", err)
	}

	// The root page must not mention the hidden or help-topic commands.
	rootPage, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash("jira/index.md")))
	if err != nil {
		t.Fatalf("reading jira/index.md: %v", err)
	}
	for _, excluded := range []string{"hidden", "help-topic"} {
		if strings.Contains(string(rootPage), excluded) {
			t.Errorf("root page must not reference excluded command %q:\n%s", excluded, rootPage)
		}
	}
}

// TestGenMarkdownTreeLinkHandlerApplied asserts that linkHandler is applied to
// the relative subcommand links in rendered output.
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

	rootPage, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash("jira/index.md")))
	if err != nil {
		t.Fatalf("reading jira/index.md: %v", err)
	}
	// The leaf child "issue" links relative to the parent's dir as "issue.md",
	// which the website handler rewrites to "./issue/".
	want := "./issue/"
	if !strings.Contains(string(rootPage), want) {
		t.Errorf("root page does not contain rewritten link %q:\n%s", want, rootPage)
	}
}

func TestGenReferenceNav(t *testing.T) {
	run := func(*cobra.Command, []string) {}
	root := &cobra.Command{Use: "jira", Run: run}
	issue := &cobra.Command{Use: "issue", Run: run}
	issue.AddCommand(&cobra.Command{Use: "create", Run: run})
	root.AddCommand(issue, &cobra.Command{Use: "me", Run: run})

	got := GenReferenceNav(root, "    ")
	want := "    { Reference = [\n" +
		"        { \"jira\" = [\n" +
		"            \"reference/jira/index.md\",\n" +
		"            { \"issue\" = [\n" +
		"                \"reference/jira/issue/index.md\",\n" +
		"                { \"create\" = \"reference/jira/issue/create.md\" },\n" +
		"            ] },\n" +
		"            { \"me\" = \"reference/jira/me.md\" },\n" +
		"        ] },\n" +
		"    ] },\n"
	if got != want {
		t.Errorf("GenReferenceNav mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSpliceReferenceNav(t *testing.T) {
	config := "head\n" + ReferenceNavStartMarker + "\nstale\nlines\n" + ReferenceNavEndMarker + "\ntail\n"
	got, err := SpliceReferenceNav(config, "    fresh\n")
	if err != nil {
		t.Fatalf("SpliceReferenceNav: %v", err)
	}
	want := "head\n" + ReferenceNavStartMarker + "\n    fresh\n" + ReferenceNavEndMarker + "\ntail\n"
	if got != want {
		t.Errorf("SpliceReferenceNav mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestSpliceReferenceNavMissingMarkers(t *testing.T) {
	if _, err := SpliceReferenceNav("no markers here\n", "x"); err == nil {
		t.Error("expected an error when markers are absent")
	}
}
