package main

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/gechr/clib/help"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira/customfield"
)

//go:embed agent_guide.md
var agentGuide string

// agentCommand groups commands curated for AI coding assistants. The schema
// and guide endpoints together give an agent everything needed to interact
// with the CLI in two calls (tree + how-to).
func agentCommand() *cobra.Command {
	cmd := groupCommand("agent", "Agent tooling: schema and guide for AI coding assistants", "agent")
	cmd.AddCommand(agentSchemaCommand())
	cmd.AddCommand(agentGuideCommand())
	cmd.AddCommand(agentADFMatrixCommand())
	cmd.AddCommand(agentFieldTypesCommand())
	return cmd
}

func agentSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Output the CLI command schema as JSON",
		Long: "Emits the full command tree, flag signatures, and per-command output " +
			"schemas for AI agent consumption. Use --output=compact for the JSON " +
			"data payload without the envelope.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeSchema(cmd)
		},
	}
	return cmd
}

func agentGuideCommand() *cobra.Command {
	sections := guideSections(agentGuide)
	cmd := &cobra.Command{
		Use:   "guide [section]",
		Short: "Print the AI-agent steering guide for jira-cli",
		Args:  cobra.MaximumNArgs(1),
		// Tab-complete the section slug. Cobra calls this with the partial
		// arg; we filter our pre-computed list and return matching slugs.
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			toComplete = strings.ToLower(toComplete)
			out := make([]string, 0, len(sections))
			for _, s := range sections {
				if toComplete == "" || strings.HasPrefix(s.slug, toComplete) {
					out = append(out, s.slug+"\t"+s.title)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := agentGuide
			if len(args) == 1 {
				section, ok := extractGuideSection(agentGuide, args[0])
				if !ok {
					// Heuristic classifier (root.outputErrorFor) picks
					// up "not found" → ErrorTypeNotFound (exit 2).
					return fmt.Errorf("guide section %q not found", args[0])
				}
				out = section
			}
			_, err := cmd.OutOrStdout().Write([]byte(out))
			return err
		},
	}
	// Custom help func: render the standard clib sections (Usage, Options)
	// then append a "Sections" block listing every parsed slug. Users see the
	// available section names instead of being told to read the guide first
	// to discover what sections it contains.
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		renderer := newHelpRenderer()
		standard := standardHelpSections(c)
		group := make(help.CommandGroup, 0, len(sections))
		for _, s := range sections {
			group = append(group, help.Command{Name: s.slug, Desc: s.title})
		}
		final := append(standard, help.Section{
			Title:   "Sections",
			Content: []help.Content{group},
		})
		// Write through the command's own output stream so a caller's
		// redirect (tests, pipes) is honored, instead of bypassing it
		// straight to os.Stdout. cobra's help-func signature returns no
		// error, so a render failure (e.g. a broken stdout pipe) is
		// surfaced on stderr rather than silently dropped.
		if err := renderer.Render(c.OutOrStdout(), final); err != nil {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "agent help: render failed: %v\n", err)
		}
	})
	return cmd
}

// agentADFMatrixCommand emits the ADF support matrix as JSON — the set
// of nodes and marks the CLI handles, for agents that want the support
// set without parsing prose.
func agentADFMatrixCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "adf-matrix",
		Short: "Emit the ADF support matrix as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows := adf.Registry().All()
			data := make([]any, 0, len(rows))
			for _, r := range rows {
				data = append(data, r)
			}
			return cmdutil.WriteEnvelope(cmd, "agent.adf-matrix", data)
		},
	}
}

// agentFieldTypesCommand emits the customfield registry as the
// envelope `data`. Same shape as `agent adf-matrix --json` so a single
// agent parser handles both surfaces.
func agentFieldTypesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fieldtypes",
		Short: "Emit the customfield type registry as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows := customfield.Registry().All()
			data := make([]any, 0, len(rows))
			for _, r := range rows {
				data = append(data, r)
			}
			return cmdutil.WriteEnvelope(cmd, "agent.fieldtypes", data)
		},
	}
	return cmd
}

// guideSection is a single ## Heading entry with both the slug used for
// matching and the title shown in help.
type guideSection struct {
	slug  string
	title string
}

// guideSections walks the embedded guide once at command construction and
// returns every `## Heading` as a (slug, title) pair. Slug is the
// lowercased title with non-alphanumeric runs collapsed to single dashes
// — what users type for `jira agent guide <slug>`.
func guideSections(doc string) []guideSection {
	var out []guideSection
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		out = append(out, guideSection{slug: slugify(title), title: title})
	}
	return out
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// extractGuideSection returns the contents of the first `## Heading` that
// matches the query — either by exact slug (e.g. "identity-setup") or by
// case-insensitive substring of the original title (e.g. "auth"). Returns
// ok=false when no heading matches. Scoped through the next `## ` heading
// or end of document.
func extractGuideSection(doc, query string) (string, bool) {
	q := strings.TrimSpace(query)
	if q == "" {
		return doc, true
	}
	qLower := strings.ToLower(q)
	qSlug := slugify(q)
	lines := strings.Split(doc, "\n")
	startIdx := -1
	for i, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		title := strings.TrimPrefix(line, "## ")
		if slugify(title) == qSlug || strings.Contains(strings.ToLower(title), qLower) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return "", false
	}
	endIdx := len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			endIdx = i
			break
		}
	}
	return strings.Join(lines[startIdx:endIdx], "\n") + "\n", true
}
