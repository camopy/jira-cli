package root

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
)

// asInputError unwraps got into a *cli.CLIInputError or fails the test.
func asInputError(t *testing.T, got error) *cli.CLIInputError {
	t.Helper()
	var ie *cli.CLIInputError
	if !errors.As(got, &ie) {
		t.Fatalf("error %v (%T) is not a *cli.CLIInputError", got, got)
	}
	return ie
}

// parseFails feeds args to a fresh command's flag set and returns the
// pflag parse error, failing the test if parsing unexpectedly succeeds.
func parseFails(t *testing.T, cmd *cobra.Command, args []string) error {
	t.Helper()
	err := cmd.Flags().Parse(args)
	if err == nil {
		t.Fatalf("parsing %v unexpectedly succeeded", args)
	}
	return err
}

func TestNewFlagErrorUnknownFlagSuggests(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("project", "", "")

	got := newFlagError(cmd, parseFails(t, cmd, []string{"--outpt=json"}))
	ie := asInputError(t, got)

	if ie.Kind != cli.InputFlagUnknown {
		t.Errorf("Kind = %d, want InputFlagUnknown", ie.Kind)
	}
	if ie.Flag != "outpt" {
		t.Errorf("Flag = %q, want outpt", ie.Flag)
	}
	if len(ie.Suggestions) != 1 || ie.Suggestions[0] != "--output" {
		t.Errorf("Suggestions = %v, want [--output]", ie.Suggestions)
	}
}

func TestNewFlagErrorUnknownShorthandSkipsSuggestion(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("output", "", "")

	got := newFlagError(cmd, parseFails(t, cmd, []string{"-Z"}))
	ie := asInputError(t, got)

	if ie.Kind != cli.InputFlagUnknown {
		t.Errorf("Kind = %d, want InputFlagUnknown", ie.Kind)
	}
	if len(ie.Suggestions) != 0 {
		t.Errorf("a shorthand group must not generate suggestions, got %v", ie.Suggestions)
	}
}

func TestNewFlagErrorValueMissing(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("project", "", "")

	got := newFlagError(cmd, parseFails(t, cmd, []string{"--project"}))
	ie := asInputError(t, got)

	if ie.Kind != cli.InputFlagValueMissing {
		t.Errorf("Kind = %d, want InputFlagValueMissing", ie.Kind)
	}
	if ie.Flag != "project" {
		t.Errorf("Flag = %q, want project", ie.Flag)
	}
}

func TestNewFlagErrorValueInvalid(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().Int("count", 0, "")

	got := newFlagError(cmd, parseFails(t, cmd, []string{"--count=notanumber"}))
	ie := asInputError(t, got)

	if ie.Kind != cli.InputFlagValueInvalid {
		t.Errorf("Kind = %d, want InputFlagValueInvalid", ie.Kind)
	}
	if ie.Flag != "count" {
		t.Errorf("Flag = %q, want count", ie.Flag)
	}
}

// TestSuggestFlagsScopesToVisibleFlags verifies suggestions are drawn from
// the command's own flags and its inherited persistent flags, and that
// single-character flags never appear as candidates.
func TestSuggestFlagsScopesToVisibleFlags(t *testing.T) {
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("profile", "", "")
	child := &cobra.Command{Use: "list"}
	child.Flags().String("project", "", "")
	child.Flags().BoolP("watch", "w", false, "")
	root.AddCommand(child)

	if got := suggestFlags(child, "porject"); len(got) != 1 || got[0] != "--project" {
		t.Errorf("suggestFlags(porject) = %v, want [--project]", got)
	}
	if got := suggestFlags(child, "porfile"); len(got) != 1 || got[0] != "--profile" {
		t.Errorf("inherited flag not suggested: got %v, want [--profile]", got)
	}
}

// TestSuggestFlagsDedupesPreservingVisitOrder pins the candidate contract:
// equal-distance suggestions keep visit order (the command's own flags before
// inherited persistent flags), and a flag visible both locally and inherited
// is offered once.
func TestSuggestFlagsDedupesPreservingVisitOrder(t *testing.T) {
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("beta1", "", "")
	root.PersistentFlags().String("shared", "", "")
	child := &cobra.Command{Use: "list"}
	child.Flags().String("beta2", "", "")
	child.Flags().String("shared", "", "")
	root.AddCommand(child)

	if got := suggestFlags(child, "beta"); !slices.Equal(got, []string{"--beta2", "--beta1"}) {
		t.Errorf("suggestFlags(beta) = %v, want [--beta2 --beta1] (own flags before inherited)", got)
	}
	if got := suggestFlags(child, "shraed"); !slices.Equal(got, []string{"--shared"}) {
		t.Errorf("suggestFlags(shraed) = %v, want the duplicated flag offered once", got)
	}
}

func TestMissingRequiredFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "create", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("note", "", "")
	if err := cmd.MarkFlagRequired("summary"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.MarkFlagRequired("project"); err != nil {
		t.Fatal(err)
	}

	missing := missingRequiredFlags(cmd)
	if len(missing) != 2 || missing[0] != "project" || missing[1] != "summary" {
		t.Errorf("missing = %v, want [project summary]", missing)
	}

	if err := cmd.Flags().Parse([]string{"--summary=x", "--project=y"}); err != nil {
		t.Fatal(err)
	}
	if missing := missingRequiredFlags(cmd); len(missing) != 0 {
		t.Errorf("after setting both, missing = %v, want none", missing)
	}
}

func TestRequiredFlagError(t *testing.T) {
	ie := asInputError(t, requiredFlagError([]string{"project", "summary"}))
	if ie.Kind != cli.InputRequiredFlagMissing {
		t.Errorf("Kind = %d, want InputRequiredFlagMissing", ie.Kind)
	}
	if ie.Flag != "project" {
		t.Errorf("Flag = %q, want the first missing flag (project)", ie.Flag)
	}
}

// TestFirstPositional verifies the command-token scan resolves flag arity
// so a value-taking flag does not have its value mistaken for a command,
// and that a probe parse error still yields the positionals scanned
// before the failure.
func TestFirstPositional(t *testing.T) {
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("profile", "", "")
	root.PersistentFlags().BoolP("debug", "d", false, "")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"plain command", []string{"issue"}, "issue"},
		{"value flag does not swallow command", []string{"--profile", "work", "issue"}, "issue"},
		{"value flag with equals", []string{"--profile=work", "issue"}, "issue"},
		{"bool flag does not consume next token", []string{"-d", "issue"}, "issue"},
		{"command trailed by a valueless flag", []string{"issue", "--profile"}, "issue"},
		{"no command token", []string{"--profile=work"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstPositional(root, c.args); got != c.want {
				t.Errorf("firstPositional(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

func TestRetypeArgValidators(t *testing.T) {
	root := &cobra.Command{Use: "jira"}
	leaf := &cobra.Command{
		Use:  "view",
		Args: cobra.ExactArgs(1),
		Run:  func(*cobra.Command, []string) {},
	}
	noValidator := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(leaf, noValidator)

	retypeArgValidators(root)

	err := leaf.Args(leaf, []string{})
	ie := asInputError(t, err)
	if ie.Kind != cli.InputArgCountInvalid {
		t.Errorf("Kind = %d, want InputArgCountInvalid", ie.Kind)
	}
	if err := leaf.Args(leaf, []string{"KEY-1"}); err != nil {
		t.Errorf("a satisfied validator must still pass, got %v", err)
	}
	if noValidator.Args != nil {
		t.Error("a command with no validator must keep Cobra's arbitrary-args default")
	}
}

func TestUnknownCommandError(t *testing.T) {
	root := &cobra.Command{Use: "jira", SuggestionsMinimumDistance: 2}
	root.AddCommand(&cobra.Command{Use: "issue", Run: func(*cobra.Command, []string) {}})

	ie := asInputError(t, unknownCommandError(root, "isue"))
	if ie.Kind != cli.InputCommandUnknown {
		t.Errorf("Kind = %d, want InputCommandUnknown", ie.Kind)
	}
	if len(ie.Suggestions) != 1 || ie.Suggestions[0] != "issue" {
		t.Errorf("Suggestions = %v, want [issue]", ie.Suggestions)
	}
}

// TestAssembledTreeRejectsUnknownFlag drives the real, fully assembled
// command tree: an unknown flag fails flag parsing, Cobra routes it
// through the root FlagErrorFunc, and ExecuteContextC returns the typed
// error — the production path, not a synthetic command.
func TestAssembledTreeRejectsUnknownFlag(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	root.SetArgs([]string{"issue", "list", "--bogusflag"})

	_, execErr := root.ExecuteContextC(context.Background())
	ie := asInputError(t, execErr)
	if ie.Kind != cli.InputFlagUnknown {
		t.Errorf("Kind = %d, want InputFlagUnknown", ie.Kind)
	}
}

// TestAssembledTreeRejectsBadArgCount drives the real tree: a command
// invoked with the wrong positional count fails its (wrapped) validator
// before RunE, so ExecuteContextC returns the typed count error.
func TestAssembledTreeRejectsBadArgCount(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	root.SetArgs([]string{"issue", "view"}) // issue view requires one key

	_, execErr := root.ExecuteContextC(context.Background())
	ie := asInputError(t, execErr)
	if ie.Kind != cli.InputArgCountInvalid {
		t.Errorf("Kind = %d, want InputArgCountInvalid", ie.Kind)
	}
}

// TestAssembledTreeRejectsUnknownCommand exercises the Execute precheck
// against the real tree: root.Find rejects an unknown top-level command,
// and the precheck's helpers turn it into a typed error with a "did you
// mean" candidate drawn from the actual command set.
func TestAssembledTreeRejectsUnknownCommand(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	args := []string{"isue"}
	if _, _, ferr := root.Find(args); ferr == nil {
		t.Fatal("root.Find should reject the unknown command \"isue\"")
	}
	ie := asInputError(t, unknownCommandError(root, firstPositional(root, args)))
	if ie.Kind != cli.InputCommandUnknown {
		t.Errorf("Kind = %d, want InputCommandUnknown", ie.Kind)
	}
	if len(ie.Suggestions) == 0 || ie.Suggestions[0] != "issue" {
		t.Errorf("Suggestions = %v, want issue first", ie.Suggestions)
	}
}

// TestAssembledTreeRejectsUnknownSubcommand exercises the group-parent
// precheck: Cobra's legacy args handling accepts a stray positional on a
// non-root group (Find returns the parent with no error), so Execute's
// guard must recognize the typo'd subcommand itself — both bare, where
// Cobra would exit 0 with usage, and with a trailing flag, where the
// parent's flag parse would report "unknown flag" and mask the real
// mistake.
func TestAssembledTreeRejectsUnknownSubcommand(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	for _, args := range [][]string{
		{"issue", "lst"},
		{"issue", "lst", "--limit", "5"},
	} {
		cmd, cargs, ferr := root.Find(args)
		if ferr != nil {
			t.Fatalf("Find(%v) errored (%v); the guard exists because Cobra accepts this", args, ferr)
		}
		if cmd.Runnable() || !cmd.HasSubCommands() {
			t.Fatalf("Find(%v) = %q; want the non-runnable issue group", args, cmd.Name())
		}
		tok := firstPositional(cmd, cargs)
		if tok != "lst" {
			t.Fatalf("firstPositional(%v) = %q, want lst", cargs, tok)
		}
		ie := asInputError(t, unknownCommandError(cmd, tok))
		if ie.Kind != cli.InputCommandUnknown {
			t.Errorf("Kind = %d, want InputCommandUnknown", ie.Kind)
		}
		if msg := ie.Error(); !strings.Contains(msg, `for "jira issue"`) {
			t.Errorf("message %q should name the group the typo sits under", msg)
		}
		if len(ie.Suggestions) == 0 || ie.Suggestions[0] != "list" {
			t.Errorf("Suggestions = %v, want list first", ie.Suggestions)
		}
	}
}

// TestFirstPositionalStopsAtTerminator pins the "--" semantics: tokens the
// caller explicitly declared non-flags are never offered up as command
// candidates, so `jira issue -- --output=json` can't be told its terminated
// token is an unknown command.
func TestFirstPositionalStopsAtTerminator(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	cmd, cargs, ferr := root.Find([]string{"issue", "--", "--output=json"})
	if ferr != nil {
		t.Fatalf("Find errored: %v", ferr)
	}
	if tok := firstPositional(cmd, cargs); tok != "" {
		t.Errorf("terminated token offered as a command candidate: %q", tok)
	}
	// A real typo before the terminator is still caught.
	cmd, cargs, _ = root.Find([]string{"issue", "lst", "--", "x"})
	if tok := firstPositional(cmd, cargs); tok != "lst" {
		t.Errorf("token before the terminator = %q, want lst", tok)
	}
}

// A foreign-CLI flag has no near-miss among the command's real flags, so
// the unknown-flag error falls back to the foreign-equivalents table for
// its suggestions.
func TestNewFlagErrorForeignFlagSuggestsEquivalents(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("output", "", "")

	got := newFlagError(cmd, parseFails(t, cmd, []string{"--plain"}))
	ie := asInputError(t, got)

	if ie.Flag != "plain" {
		t.Errorf("Flag = %q, want plain", ie.Flag)
	}
	want := []string{"--output=human", "--output=json"}
	if !slices.Equal(ie.Suggestions, want) {
		t.Errorf("Suggestions = %v, want %v", ie.Suggestions, want)
	}
}
