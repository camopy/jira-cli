package agent

import (
	"embed"
	"fmt"
	"strings"

	"github.com/gechr/clib/help"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/schema"
	"github.com/matcra587/jira-cli/internal/jira/customfield"
)

//go:embed guide
var guideFS embed.FS

// workflowGoals is populated by init from each guide/<slug>.md file's
// `Goal: …` line. Used by `agent guide --help` so the Sections block
// shows what every workflow is for rather than echoing the slug twice.
var workflowGoals = map[string]string{}

// workflowOrder is the canonical ordering used by the concatenated
// guide output and the `agent guide --help` Sections block. Each entry
// MUST have a matching guide/<slug>.md file; an init-time check fails
// fast on drift between this list and the embedded directory so a typo
// here or a stray file there is caught at binary startup, not at first
// guide invocation.
var workflowOrder = []string{
	"core_contract",
	"identity_setup",
	"find_user",
	"auth_setup",
	"inspect_schema",
	"configure_editor",
	"safe_mutation",
	"security_posture",
	"read_issue",
	"list_issues",
	"search_jql",
	"discover_board",
	"cache_metadata",
	"create_issue",
	"create_subtask",
	"edit_issue",
	"transition_issue",
	"add_comment",
	"list_comments",
	"attach_file",
	"manage_watchers",
	"link_issues",
	"add_weblink",
	"log_work",
	"clone_issue",
	"move_issue",
	"rank_issues",
	"delete_issue",
	"self_update",
	"author_adf",
	"adf_reference",
	"jql_reference",
}

// NewCommand groups commands curated for AI coding assistants. The schema
// and guide endpoints together give an agent everything needed to interact
// with the CLI in two calls (tree + how-to).
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("agent", "Agent tooling: schema and guide for AI coding assistants", "agent")
	cmd.Long = "Emit the machine-readable contract that AI coding assistants consume. `jira " +
		"agent schema` prints the full command and envelope schema, `jira agent guide` prints " +
		"the embedded usage guide, and `jira agent adf-matrix` / `jira agent fieldtypes` " +
		"describe rich-text and custom-field handling.\n\n" +
		"These are the authoritative behavior spec: an agent reads them to learn the flags, " +
		"output shapes, and exit codes before driving the CLI."
	cmd.Example = `$ jira agent schema

# Read one guide section
$ jira agent guide core_contract`
	cmd.AddCommand(agentSchemaCommand())
	cmd.AddCommand(agentGuideCommand())
	cmd.AddCommand(agentADFMatrixCommand())
	cmd.AddCommand(agentFieldTypesCommand())
	return cmd
}

func agentSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Emit the CLI command schema as JSON",
		Long: "Emits the full command tree, flag signatures, and per-command output " +
			"schemas for AI agent consumption. Use `--output=compact` for the JSON " +
			"data payload without the envelope.",
		Example: `$ jira agent schema

# Drop the envelope and print only the JSON data payload
$ jira agent schema --output=compact`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return schema.WriteSchema(cmd)
		},
	}
	return cmd
}

func agentGuideCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guide [section]",
		Short: "Print the AI-agent steering guide",
		Long: "Print embedded runbooks for using jira-cli from scripts and AI coding agents. " +
			"Use it when an automated workflow needs the command contract, safe mutation " +
			"rules, or task-specific guidance without reading the source tree.\n\n" +
			"With no section, the command prints every guide in the canonical order. Pass a " +
			"section slug, such as `safe_mutation`, to print only that workflow.",
		Example: `$ jira agent guide

# Print only one workflow section
$ jira agent guide safe_mutation`,
		Args: cobra.MaximumNArgs(1),
		// Tab-complete the workflow slug. Cobra calls this with the
		// partial arg; we filter the canonical order list and return
		// matching slugs.
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			toComplete = strings.ToLower(toComplete)
			out := make([]string, 0, len(workflowOrder))
			for _, slug := range workflowOrder {
				if toComplete == "" || strings.HasPrefix(slug, toComplete) {
					out = append(out, slug+"\t"+slug)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var out string
			if len(args) == 1 {
				section, ok := loadGuideSection(args[0])
				if !ok {
					// A bad section slug is bad command-line input
					// (validation, exit 3) — nothing was looked up in
					// Jira, so not_found would be misleading.
					return cli.NewCLIInputError(cli.InputArgValueInvalid, fmt.Sprintf("guide section %q not found", args[0]))
				}
				out = section
			} else {
				out = loadGuide()
			}
			_, err := cmd.OutOrStdout().Write([]byte(out))
			return err
		},
	}
	// Custom help func: render the standard clib sections (Usage,
	// Options) then append a "Sections" block listing every workflow
	// slug. Users see the available section names instead of being
	// told to read the guide first to discover what sections exist.
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		renderer := cmdutil.NewHelpRenderer()
		standard := cmdutil.StandardHelpSections(c)
		group := make(help.CommandGroup, 0, len(workflowOrder))
		for _, slug := range workflowOrder {
			group = append(group, help.Command{Name: slug, Desc: workflowGoals[slug]})
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

// NewADFMatrixCommand exposes the ADF support matrix command for mounting
// under the docent agent group.
func NewADFMatrixCommand() *cobra.Command {
	return agentADFMatrixCommand()
}

// NewFieldTypesCommand exposes the customfield registry command for
// mounting under the docent agent group.
func NewFieldTypesCommand() *cobra.Command {
	return agentFieldTypesCommand()
}

// agentADFMatrixCommand emits the ADF support matrix as JSON — the set
// of nodes and marks the CLI handles, for agents that want the support
// set without parsing prose.
func agentADFMatrixCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "adf-matrix",
		Short: "Emit the ADF support matrix as JSON",
		Long: "Print the Atlassian Document Format (ADF) nodes and marks that jira-cli knows " +
			"how to read or write — the support matrix. ADF is the JSON rich-text format Jira " +
			"uses for descriptions, comments, worklogs, and rich-text custom fields; reach for " +
			"it before generating ADF by hand. The matrix covers every node and mark of the " +
			"pinned ADF schema, one row each, recording which operations the CLI supports " +
			"(author, render, preserve, validate, submit); each row carries an `official_url` " +
			"to the matching Atlassian reference.\n\n" +
			"Rows come in two status tiers. `mvp` rows are the curated core: authorable " +
			"(most from Markdown) and rendered in human output. `preserve-only` rows validate, " +
			"submit, and round-trip as native ADF, but have no Markdown authoring surface and " +
			"render as their child text only — author them as native ADF via --json-input.\n\n" +
			"The output is local registry data: it does not contact Jira, and it does not prove " +
			"that a particular Jira field accepts every listed node.\n\n" +
			"See the Atlassian ADF structure reference: " +
			"<https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/>",
		Example: `# Inspect local ADF support without contacting Jira
$ jira agent adf-matrix

# Emit the matrix as JSON for an agent
$ jira agent adf-matrix --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdutil.WriteEnvelope(cmd, "agent.adf-matrix", adf.Registry().All())
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
		Long: "Print jira-cli's local registry for Jira custom field schema types. Use it when " +
			"building JSON payloads for create, edit, clone, or move commands and you need " +
			"to know how a field value is encoded.\n\n" +
			"The registry is a CLI encoding guide, not a live field list. Combine it with " +
			"`jira issue create --field-help` or `jira issue edit --field-help` when you " +
			"need the fields configured for one Jira project or issue.",
		Example: `# Inspect local custom field encoding support
$ jira agent fieldtypes

# Emit the registry as JSON for an agent
$ jira agent fieldtypes --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdutil.WriteEnvelope(cmd, "agent.fieldtypes", customfield.Registry().All())
		},
	}
	return cmd
}

// loadGuide assembles the preamble plus every workflow file in
// workflowOrder, separated by a single blank line. The init() check
// guarantees every workflowOrder slug has a corresponding file, so the
// inner ReadFile is not expected to fail at runtime. The contract-version
// line is rendered from the schema constant rather than baked into the
// preamble so guide and schema can never report different versions.
func loadGuide() string {
	var b strings.Builder
	if data, err := guideFS.ReadFile("guide/_preamble.md"); err == nil {
		b.Write(data)
	}
	b.WriteString("\nContract version: " + schema.ContractVersion +
		" — the same revision `jira agent schema` reports at `data.schema_version`; see the core_contract section for the scheme.\n")
	for _, slug := range workflowOrder {
		data, err := guideFS.ReadFile("guide/" + slug + ".md")
		if err != nil {
			continue
		}
		b.WriteString("\n")
		b.Write(data)
	}
	return b.String()
}

// loadGuideSection returns one workflow's runbook by slug. Lookup is
// exact first (canonical underscore form, also accepts the dash form
// the help block displays), then case-insensitive substring on the
// slug list so `agent guide auth` resolves to `auth_setup`.
func loadGuideSection(query string) (string, bool) {
	q := strings.TrimSpace(query)
	if q == "" {
		return loadGuide(), true
	}
	qSlug := normalizeSlug(q)
	for _, slug := range workflowOrder {
		if slug == qSlug {
			data, err := guideFS.ReadFile("guide/" + slug + ".md")
			if err != nil {
				return "", false
			}
			return string(data), true
		}
	}
	qLower := strings.ToLower(q)
	for _, slug := range workflowOrder {
		if strings.Contains(slug, qLower) {
			data, err := guideFS.ReadFile("guide/" + slug + ".md")
			if err != nil {
				return "", false
			}
			return string(data), true
		}
	}
	return "", false
}

// extractGoal pulls the `Goal: …` (or `When to use this: …` on
// reference sections) one-liner from a workflow file's header, so the
// `agent guide --help` Sections block carries something more
// informative than the slug itself.
func extractGoal(slug string) string {
	data, err := guideFS.ReadFile("guide/" + slug + ".md")
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(data), "\n", 12) {
		for _, prefix := range []string{"Goal:", "When to use this:"} {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				return strings.TrimSpace(rest)
			}
		}
	}
	return ""
}

// normalizeSlug folds a user-typed query ("auth-setup", "Auth_Setup",
// "AUTH SETUP") into the canonical underscore form used by file
// names. Non-alphanumeric runs collapse to a single underscore.
func normalizeSlug(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// init fails fast if workflowOrder and guide/*.md drift apart. A
// missing or extra file is a binary-startup error so misses are caught
// in CI / on first build rather than at the first `agent guide` call.
func init() {
	entries, err := guideFS.ReadDir("guide")
	if err != nil {
		panic(fmt.Sprintf("agent: read guide dir: %v", err))
	}
	have := make(map[string]bool, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		slug := strings.TrimSuffix(name, ".md")
		if slug == "_preamble" {
			continue
		}
		have[slug] = true
	}
	want := make(map[string]bool, len(workflowOrder))
	for _, slug := range workflowOrder {
		want[slug] = true
		if !have[slug] {
			panic("agent: workflowOrder lists " + slug + " but guide/" + slug + ".md is missing")
		}
		workflowGoals[slug] = extractGoal(slug)
	}
	for slug := range have {
		if !want[slug] {
			panic("agent: guide/" + slug + ".md exists but " + slug + " is not in workflowOrder")
		}
	}
}
