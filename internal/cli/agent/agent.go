package agent

import (
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira/customfield"
)

// NewADFMatrixCommand exposes the ADF support matrix command for mounting
// under the docent agent group.
func NewADFMatrixCommand() *cobra.Command {
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

// NewFieldTypesCommand exposes the customfield registry command for
// mounting under the docent agent group. Same envelope shape as
// `agent adf-matrix` so a single agent parser handles both surfaces.
func NewFieldTypesCommand() *cobra.Command {
	return &cobra.Command{
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
}
