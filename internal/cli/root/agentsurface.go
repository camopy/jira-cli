package root

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	clogtheme "github.com/gechr/clog/theme"
	"github.com/matcra587/docent"
	docentcobra "github.com/matcra587/docent/cobra"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/matcra587/jira-cli/internal/agentguides"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/agent"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/schema"
	"github.com/matcra587/jira-cli/internal/config"
)

// mountAgentSurface builds the docent config from the fully assembled
// command tree and mounts the agent command group (hidden — it exists for
// agents and CI, not for humans browsing --help) plus the top-level
// `jira guide` human door. Must run after every other command is
// registered: the schema is a walk of the live tree, and anything mounted
// later would be invisible to it.
func mountAgentSurface(root *cobra.Command) {
	guides, err := agentguides.Load()
	if err != nil {
		// Guide drift is a build invariant, not a runtime condition: the
		// files are embedded and validated by contract tests, so a load
		// failure means a broken build.
		panic(fmt.Sprintf("root: load agent guides: %v", err))
	}

	tree := pruneHiddenFlags(root, docentcobra.Tree(root))
	tree = schema.DocentRegistry().Apply(tree)
	tree.Extensions = map[string]any{
		"envelope": "Success and error envelopes, exit codes, and output modes " +
			"are specified by the core-contract guide (`jira agent guide core-contract`).",
		// The tool-wide envelope and error schemas describe every
		// response, so they live on the root rather than any command.
		"output_contract": schema.OutputContract(),
		"exit_codes": map[string]any{
			"ok": 0, "auth": 1, "not_found": 2, "validation": 3,
			"rate_limit": 4, "server": 5, "canceled": 6, "timeout": 7,
		},
		"env": map[string]any{
			"JIRA_READ_ONLY":      "Block all Jira writes at the HTTP transport.",
			"JIRA_ADF_STRICT":     "Fail lossy ADF conversions instead of warning.",
			"JIRA_MAX_RETRY_WAIT": "Default for --max-retry-wait.",
		},
	}

	cfg := docent.Config{
		Guides:          guides,
		Command:         tree,
		ContractVersion: agentguides.ContractVersion,
	}

	agentCmd := docentcobra.NewCommand(
		cfg,
		docentcobra.WithExtraCommands(
			agent.NewADFMatrixCommand(),
			agent.NewFieldTypesCommand(),
		),
		docentcobra.WithSkillNameQualifier("jira"),
	)
	agentCmd.Hidden = true
	agentCmd.GroupID = "agent"

	humanGuide := newHumanGuideCommand(cfg)
	root.AddCommand(agentCmd, humanGuide)

	// Docent registers flag completions through cobra's
	// RegisterFlagCompletionFunc, but jira-cli's completion is clib-driven and
	// never invokes those callbacks — so bridge them into clib metadata.
	pathDescriptions := commandPathDescriptions(tree)
	bridgeDocentCompletions(agentCmd, pathDescriptions)
	bridgeDocentCompletions(humanGuide, pathDescriptions)
}

// bridgeDocentCompletions walks cmd and its descendants and copies each flag's
// cobra-registered completion values into clib enum metadata, so the
// clib-driven completion path offers them. jira-cli never calls cobra's
// completion callbacks, so docent's completions on --format, --scope,
// --harness, --section, and --path would otherwise be silently dead.
//
// The completion functions are called once, here at mount time. Docent's
// completion values are static per build — format names, scope and harness
// values, guide section headings, and schema command paths are all fixed by
// the loaded config, with no runtime input (args, toComplete) that would change
// them — so a single snapshot captures the full value set. Each value also
// receives clib EnumTerse metadata so Fish and other shells show a useful
// value-specific description instead of repeating the flag's full help text.
func bridgeDocentCompletions(cmd *cobra.Command, pathDescriptions map[string]string) {
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		completer, ok := cmd.GetFlagCompletionFunc(f.Name)
		if !ok {
			return
		}
		values, _ := completer(cmd, nil, "")
		enum, enumTerse := cleanCompletionValues(values)
		if len(enum) > 0 {
			fallback := docentCompletionTerse(cmd, f.Name, enum, pathDescriptions)
			for i := range enumTerse {
				if enumTerse[i] == "" && i < len(fallback) {
					enumTerse[i] = fallback[i]
				}
			}
			cmdutil.ExtendFlagEnum(f, enum, enumTerse)
		}
	})
	for _, child := range cmd.Commands() {
		bridgeDocentCompletions(child, pathDescriptions)
	}
}

// cleanCompletionValues normalizes cobra's "choice<TAB>description" completion
// strings into parallel clib Enum and EnumTerse slices, discarding empty
// choices while retaining any description an upstream completer supplied.
func cleanCompletionValues(values []string) (enum, enumTerse []string) {
	enum = make([]string, 0, len(values))
	enumTerse = make([]string, 0, len(values))
	for _, v := range values {
		choice, description, _ := strings.Cut(v, "\t")
		if choice != "" {
			enum = append(enum, choice)
			enumTerse = append(enumTerse, description)
		}
	}
	return enum, enumTerse
}

func docentCompletionTerse(
	cmd *cobra.Command,
	flag string,
	enum []string,
	pathDescriptions map[string]string,
) []string {
	descriptions := make([]string, len(enum))
	for i, value := range enum {
		switch {
		case cmd.Name() == "export" && flag == "format":
			descriptions[i] = map[string]string{
				"agent-skill":  "portable Agent Skills format",
				"claude-skill": "Claude Code skill format",
			}[value]
			if descriptions[i] == "" {
				descriptions[i] = "export as " + value
			}
		case cmd.Name() == "export" && flag == "scope":
			descriptions[i] = map[string]string{
				"project": "project-local skills directory",
				"user":    "user-level skills directory",
			}[value]
			if descriptions[i] == "" {
				descriptions[i] = value + " skills directory"
			}
		case cmd.Name() == "export" && flag == "harness":
			descriptions[i] = map[string]string{
				"claude-code": "Claude Code conventions",
				"codex":       "Codex conventions",
			}[value]
			if descriptions[i] == "" {
				descriptions[i] = value + " conventions"
			}
		case cmd.Name() == "guide" && flag == "section":
			descriptions[i] = map[string]string{
				"Decide":        "choose the approach",
				"Run":           "execute the workflow",
				"Save":          "capture outputs and reusable state",
				"Preconditions": "check required state",
				"Recover":       "handle failures",
				"Next":          "continue to related guidance",
			}[value]
			if descriptions[i] == "" {
				descriptions[i] = "read the " + strings.ToLower(value) + " section"
			}
		case cmd.Name() == "schema" && flag == "path":
			descriptions[i] = pathDescriptions[value]
			if descriptions[i] == "" {
				descriptions[i] = "schema for " + value
			}
		}
	}
	return descriptions
}

func commandPathDescriptions(tree docent.Command) map[string]string {
	descriptions := map[string]string{}
	var walk func(docent.Command)
	walk = func(cmd docent.Command) {
		if cmd.Path != "" {
			descriptions[cmd.Path] = cmd.Description
		}
		for _, child := range cmd.Children {
			walk(child)
		}
	}
	walk(tree)
	return descriptions
}

// pruneHiddenFlags drops hidden flags from the docent schema IR. Hidden
// flags are deliberately unadvertised surface — deprecated Markdown
// aliases keep working for old scripts, but no discovery output may teach
// a new caller to use one. Docent's tree walker carries every flag, so
// the filtering is this host's decision, made here.
func pruneHiddenFlags(cmd *cobra.Command, tree docent.Command) docent.Command {
	hidden := map[string]bool{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			hidden[f.Name] = true
		}
	})
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			hidden[f.Name] = true
		}
	})
	if len(hidden) > 0 {
		kept := tree.Flags[:0:0]
		for _, flag := range tree.Flags {
			if !hidden[flag.Name] {
				kept = append(kept, flag)
			}
		}
		tree.Flags = kept
	}
	byName := map[string]*cobra.Command{}
	for _, child := range cmd.Commands() {
		byName[child.Name()] = child
	}
	for i, child := range tree.Children {
		if cobraChild, ok := byName[child.Name]; ok {
			tree.Children[i] = pruneHiddenFlags(cobraChild, child)
		}
	}
	return tree
}

// newHumanGuideCommand mounts the human door on the same guide set. Docent
// emits plain Markdown; styling is this host's job, and only an
// interactive human terminal gets it — agents and pipes receive the raw
// bytes byte-identical to `jira agent guide`. Glamour styles complete
// documents, so the seam is buffer-then-render, never a streaming writer.
func newHumanGuideCommand(cfg docent.Config) *cobra.Command {
	var buf bytes.Buffer
	cfg.Out = &buf
	cmd := docentcobra.NewGuideCommand(cfg)
	cmd.GroupID = "agent"
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		buf.Reset()
		if err := run(c, args); err != nil {
			return err
		}
		det := cmdutil.DetectorFromContext(c)
		if !det.IsTTY || det.Mode != cli.ModePlain {
			_, err := c.OutOrStdout().Write(buf.Bytes())
			return err
		}
		_, err := io.WriteString(c.OutOrStdout(), styleGuideMarkdown(c, buf.String()))
		return err
	}
	return cmd
}

// styleGuideMarkdown renders guide Markdown for an interactive terminal.
// The no-slug index is key:value lines — an agent shape, not prose — so it
// is reshaped into real Markdown first. Rendering failures fall back to
// the raw bytes: a styling problem must never cost the content.
func styleGuideMarkdown(cmd *cobra.Command, md string) string {
	if strings.HasPrefix(md, "# Agent Guide Index") {
		md = guideIndexToMarkdown(md)
	}
	// Wrap at min(terminal, 80): guide prose is authored at 80 columns and
	// glamour preserves the hard breaks inside list items, so reflowing
	// paragraphs any wider would give lists and paragraphs two different
	// measures on a wide terminal.
	wrap := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && w < wrap {
		wrap = w
	}
	opts := []glamour.TermRendererOption{
		glamour.WithStandardStyle(glamourStyleName(cmd)),
		glamour.WithWordWrap(wrap),
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return md
	}
	styled, err := r.Render(md)
	if err != nil {
		return md
	}
	return styled
}

// glamourStyleName maps the CLI's resolved theme onto a glamour style, so
// the guide view follows the same JIRA_THEME / theme.name decision as
// every other styled surface. A theme glamour also ships (dark, light,
// dracula, tokyo-night) is used by name; anything else falls back to the
// theme's light or dark background.
func glamourStyleName(cmd *cobra.Command) string {
	name := strings.TrimSpace(os.Getenv(config.EnvThemeName))
	if name == "" {
		if cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd))); err == nil {
			name = strings.TrimSpace(cfg.Theme.Name)
		}
	}
	name = strings.ToLower(name)
	if _, ok := styles.DefaultStyles[name]; ok {
		return name
	}
	if cmdutil.HumanJSONPrintTheme(cmd).Background == clogtheme.BackgroundLight {
		return styles.LightStyle
	}
	return styles.DarkStyle
}

// guideIndexToMarkdown reshapes the frontmatter-style guide index into
// Markdown a human can scan: each guide becomes a titled section naming
// its `jira guide <slug>` invocation. The slug line precedes the title in
// the index, so it is buffered until the title arrives.
func guideIndexToMarkdown(index string) string {
	var b strings.Builder
	slug := ""
	for line := range strings.Lines(index) {
		line = strings.TrimSuffix(line, "\n")
		key, value, isKV := strings.Cut(line, ": ")
		switch {
		case strings.HasPrefix(line, "# Agent Guide Index"), line == "", !isKV:
			fmt.Fprintf(&b, "%s\n", line)
		case key == "slug":
			slug = value
		case key == "title":
			fmt.Fprintf(&b, "## %s\n\n`jira guide %s`\n\n", value, slug)
		case key == "description":
			fmt.Fprintf(&b, "%s\n\n", value)
		case key == "when_to_use":
			fmt.Fprintf(&b, "**When:** %s\n\n", value)
		case key == "commands":
			if value != "" {
				fmt.Fprintf(&b, "**Commands:** `%s`\n", value)
			}
		case key == "order":
			// Canonical-ordering metadata; the listing is already ordered.
		default:
			fmt.Fprintf(&b, "**%s:** %s\n", strings.ReplaceAll(key, "_", " "), value)
		}
	}
	return b.String()
}

// writeDiscoverySchema emits the schema for the bare-`jira` non-TTY
// contract by dispatching to this root's own mounted `agent schema`
// command — byte-identical by construction, with no shared state between
// root instances (each root carries its own mount).
func writeDiscoverySchema(cmd *cobra.Command) error {
	schemaCmd, _, err := cmd.Root().Find([]string{"agent", "schema"})
	if err != nil {
		return fmt.Errorf("locate agent schema command: %w", err)
	}
	schemaCmd.SetOut(cmd.OutOrStdout())
	schemaCmd.SetErr(cmd.ErrOrStderr())
	return schemaCmd.RunE(schemaCmd, nil)
}
