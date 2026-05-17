// `jira issue link` command tree.
//
// Sub-command group:
//   - link list KEY            — flattened inward+outward array, sorted
//   - link delete KEY LINK_ID  — DELETE /issueLink/{id}, force-gated
//   - link types               — instance link-type list, cache-backed
//
// Default action (no subcommand): `link KEY --to OTHER --type NAME`
// retains today's create form for back-compat.
package main

import (
	"encoding/json"
	"fmt"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// issueLinkSubCommand builds the `jira issue link` cobra tree.
//
// The legacy `link KEY --to OTHER --type NAME` create form lives at
// the top level (cobra's `RunE`) so today's invocation keeps working
// alongside the new `link list/delete/types` sub-commands.
func issueLinkSubCommand() *cobra.Command {
	var to, linkType string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "link KEY",
		Short: "Inspect, create, or remove issue links",
		Long: `Manage Jira issue links.

Sub-commands:
  list KEY            List the links on an issue (inward + outward)
  delete KEY LINK_ID  Remove a link by its global id
  types               Show the configured link types in this Jira

Default action — no sub-command:
  jira issue link KEY --to OTHER --type NAME
  Creates a link of the given type. KEY is the inward issue (the one
  whose link page will show the relationship); --to is the outward
  issue. Semantics depend on --type:
    Blocks  — KEY blocks --to
    Relates — KEY relates to --to
    Cloners — KEY clones --to (or "is cloned by", per direction)`,
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if to == "" || linkType == "" {
				return fmt.Errorf("validation: --to and --type are required")
			}
			if dryRun {
				return writeEnvelope(cmd, "issue.link", map[string]any{
					"inward_issue": args[0], "outward_issue": to, "type": linkType, "dry_run": true,
				})
			}
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link")
			}
			resp, err := jira.NewIssueLinkService(client).Create(cmd.Context(), &jira.IssueLinkRequest{
				Type: linkType, InwardIssue: args[0], OutwardIssue: to,
			})
			if err != nil {
				return err
			}
			return writeEnvelopeWithResponse(cmd, "issue.link", map[string]any{
				"inward_issue": args[0], "outward_issue": to, "type": linkType, "dry_run": false,
			}, resp)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Outward issue key")
	cmd.Flags().StringVar(&linkType, "type", "", "Link type name (Blocks, Relates, Cloners, …)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without creating the link")
	// A link needs both endpoints: passing one of --to / --type without
	// the other is always a syntax error. Declared as Cobra metadata so
	// the half-specified link is rejected before RunE.
	cmd.MarkFlagsRequiredTogether("to", "type")
	// --type completion driven by the cachelinktype predictor.
	// Cache primer: `jira cache linktypes`.
	clib.Extend(cmd.Flags().Lookup("type"), clib.FlagExtra{Placeholder: "NAME", Complete: "predictor=cachelinktype"})

	cmd.AddCommand(issueLinkListCommand())
	cmd.AddCommand(issueLinkDeleteCommand())
	cmd.AddCommand(issueLinkTypesCommand())
	return cmd
}

// issueLinkListCommand wires `jira issue link list KEY`.
//
// Derives from `GET /issue/{key}?fields=issuelinks`; flattens the
// inward/outward fork into a single direction-aware array; sorts by
// (direction, type.name, other_issue.key) ASC.
//
// `--raw` returns Atlassian's `issuelinks` array verbatim.
func issueLinkListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list KEY",
		Short:       "List the links on an issue (inward + outward)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link.list")
			}
			links, _, err := jira.NewIssueLinkService(client).List(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeEnvelope(cmd, "issue.link.list", map[string]any{
				"key":   args[0],
				"links": links,
				"count": len(links),
			})
		},
	}
	return cmd
}

// issueLinkDeleteCommand wires `jira issue link delete KEY LINK_ID`.
//
// `KEY` is required positionally for symmetry with other commands and
// to carry `dynamic-args='issuekey'`, but only the link id is sent on
// the wire — `DELETE /issueLink/{id}` is a global endpoint. Force-gated
// under `--no-input`. `--dry-run` skips the HTTP call.
func issueLinkDeleteCommand() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:         "delete KEY LINK_ID",
		Short:       "Remove an issue link by id",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := noInputRequested(cmd)
			key, linkID := args[0], args[1]
			if dryRun {
				return writeEnvelope(cmd, "issue.link.delete", map[string]any{
					"key":     key,
					"link_id": linkID,
					"dry_run": true,
				})
			}
			det := DetectorFromContext(cmd)
			if !force {
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("issue link delete requires --force in headless / agent / --no-input mode")
				}
				if ok, err := confirmDestructive(cmd, "link delete", linkID); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "link delete", nil)
				}
			}
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link.delete")
			}
			resp, err := jira.NewIssueLinkService(client).Delete(cmd.Context(), linkID)
			if err != nil {
				return err
			}
			// data.link_id MUST echo the supplied id verbatim
			// regardless of the source KEY — links are global; the
			// CLI is explicit about which id was removed.
			return writeEnvelopeWithResponse(cmd, "issue.link.delete", map[string]any{
				"key":     key,
				"link_id": linkID,
				"deleted": true,
				"dry_run": false,
			}, resp)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive removal (required under --no-input)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without removing the link")
	return cmd
}

// issueLinkTypesCommand wires `jira issue link types`.
//
// Reads from the `linktypes` cache when fresh; primes via
// /issueLinkType when stale or `--refresh` is supplied. Default TTL
// 60 minutes.
//
// `--raw` returns Atlassian's native `{issueLinkTypes: [...]}`
// envelope verbatim.
func issueLinkTypesCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "types",
		Short: "Show the configured link types in this Jira instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute
			data, fromCache, fetchedAt, cacheSourceState, err := cacheReadOrFetch(cacheKeyForProfile(cmd, profile), "linktypes", ttl, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for issue.link.types")
				}
				return fetchLinkTypesForCache(cmd, client)
			})
			if err != nil {
				return err
			}
			var types []jira.IssueLinkType
			if err := json.Unmarshal(data, &types); err != nil {
				return fmt.Errorf("issue.link.types: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"link_types": types,
				"count":      len(types),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			addCacheStateFields(envelopeData, cacheSourceState, len(types))
			return writeEnvelope(cmd, "issue.link.types", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	return cmd
}

// fetchLinkTypesForCache calls /issueLinkType, unwraps the payload,
// and returns the JSON-encoded slice. Reused by the cache primer
// command in cache.go.
func fetchLinkTypesForCache(cmd *cobra.Command, client *jira.Client) (json.RawMessage, error) {
	types, _, err := jira.NewIssueLinkTypeService(client).List(cmd.Context())
	if err != nil {
		return nil, err
	}
	return marshalNonNilSlice(types)
}
