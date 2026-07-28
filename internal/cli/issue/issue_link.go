// `jira issue link` command tree.
//
// Sub-command group:
//   - link list KEY            — flattened inward+outward array, sorted
//   - link delete KEY LINK_ID  — DELETE /issueLink/{id}, force-gated
//   - link types               — instance link-type list, cache-backed
//
// Default action (no subcommand): `link KEY --to OTHER --type NAME`
// retains today's create form for back-compat.

package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	xslices "github.com/gechr/x/slices"
	xstrings "github.com/gechr/x/strings"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	cachereg "github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/envelope"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
)

// issueLinkSubCommand builds the `jira issue link` cobra tree.
//
// The legacy `link KEY --to OTHER --type NAME` create form lives at
// the top level (cobra's `RunE`) so today's invocation keeps working
// alongside the new `link list/delete/types` sub-commands.
func issueLinkSubCommand() *cobra.Command {
	var to, linkType, jsonInput string
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "link KEY...",
		Short: "Manage issue links",
		Long: "Create, list, delete, and discover Jira issue links. Use the default action " +
			"`jira issue link KEY --to OTHER --type NAME` to create a link, or use the " +
			"subcommands for reads and deletes.\n\n" +
			"For create, `KEY` is the inward issue and `--to` is the outward issue. Link " +
			"type semantics come from Jira, so confirm the configured type names with " +
			"`jira issue link types` when direction matters. The create output's " +
			"`preview` sentences show the line each issue's page will render — the " +
			"inward issue displays the type's outward phrase, not its inward one — so " +
			"check them before trusting the direction.\n\n" +
			"`--json-input` accepts the native POST /rest/api/3/issueLink body — `type`, " +
			"`inwardIssue`, `outwardIssue`, and an optional `comment` — so an API-shaped " +
			"payload needs no translation to flags.\n\n" +
			"`--dry-run` previews create requests without contacting Jira. Link deletes " +
			"are force-gated in headless, agent, and `--no-input` mode.",
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Args:        cobra.ArbitraryArgs,
		Example: `# Mark one issue as blocking another
$ jira issue link PROJ-123 --to PROJ-456 --type Blocks

# Run 'jira issue link types' for the link types your site allows
$ jira issue link PROJ-123 --to PROJ-456 --type Relates

# Create a link from a native Jira REST body
$ jira issue link --json-input link.json

# Preview a link creation without contacting Jira
$ jira issue link PROJ-123 --to PROJ-456 --type Blocks --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && jsonInput == "" {
				return cmd.Help()
			}
			var in issueLinkCreateInput
			var keys []string
			var err error
			if jsonInput != "" {
				in, keys, err = issueLinkInputFromJSON(jsonInput, args)
			} else {
				keys, err = issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
				if err == nil && xstrings.AnyEmpty(to, linkType) {
					err = fmt.Errorf("validation: --to and --type are required (or supply the native body via --json-input)")
				}
				in = issueLinkCreateInput{To: to, Type: linkType}
			}
			if err != nil {
				return err
			}
			in.DryRun = dryRun
			in.Command = "issue.link"
			if len(keys) > 1 {
				return runIssueLinkCreateMany(cmd, keys, parallelism, in)
			}
			if dryRun {
				in.previewType = cachedLinkTypeForPreview(cmd, in)
				return cmdutil.WriteEnvelope(cmd, "issue.link", issueLinkCreateData(keys[0], in, true))
			}
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link")
			}
			in.previewType = liveLinkTypeForPreview(cmd, client, profile, in)
			var resp *jira.Response
			if err := cmdutil.Spin(cmd, "issue.link", func(ctx context.Context) error {
				var e error
				resp, e = cmdutil.ServicesForClient(client).IssueLink().Create(ctx, issueLinkRequestFor(keys[0], in))
				return e
			}); err != nil {
				return err
			}
			return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.link", issueLinkCreateData(keys[0], in, false), resp)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &to, "to", "", "Outward issue key", clib.FlagExtra{Group: "Link", Placeholder: "KEY", Complete: "predictor=issuekey"})
	// --type completion driven by the cachelinktype predictor.
	// Cache primer: `jira cache linktypes`.
	cmdutil.AddStringVar(cmd.Flags(), &linkType, "type", "", "Link type name (Blocks, Relates, Cloners, …)", clib.FlagExtra{Group: "Link", Placeholder: "NAME", Complete: "predictor=cachelinktype"})
	cmdutil.AddFileFlag(cmd.Flags(), &jsonInput, "json-input", "", "Read the native issueLink create body from a JSON file (type, inwardIssue, outwardIssue, comment)", "Input", "FILE")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without creating the link")
	// A link needs both endpoints: passing one of --to / --type without
	// the other is always a syntax error. Declared as Cobra metadata so
	// the half-specified link is rejected before RunE. The native body
	// carries its own endpoints, so it excludes both flags.
	cmd.MarkFlagsRequiredTogether("to", "type")
	cmd.MarkFlagsMutuallyExclusive("json-input", "to")
	cmd.MarkFlagsMutuallyExclusive("json-input", "type")
	cmdutil.AddParallelismFlag(cmd, &parallelism)

	cmd.AddCommand(issueLinkListCommand())
	cmd.AddCommand(issueLinkDeleteCommand())
	cmd.AddCommand(issueLinkTypesCommand())
	return cmd
}

type issueLinkCreateInput struct {
	To   string
	Type string
	// TypeID addresses the link type by id when the native body uses
	// {"type": {"id": ...}} instead of a name.
	TypeID string
	// Comment is the native REST comment block (ADF body already
	// validated), forwarded verbatim with the link.
	Comment map[string]any
	Command string
	DryRun  bool
	// previewType carries the resolved link type's phrase pair when it is
	// known, so the create envelope can render the sentence each endpoint's
	// page will display. nil degrades to no preview, never an error.
	previewType *jira.IssueLinkType
}

// issueLinkRequestFor builds the wire request for one inward issue key.
func issueLinkRequestFor(key string, in issueLinkCreateInput) *jira.IssueLinkRequest {
	return &jira.IssueLinkRequest{
		Type:         in.Type,
		TypeID:       in.TypeID,
		InwardIssue:  key,
		OutwardIssue: in.To,
		Comment:      in.Comment,
	}
}

// issueLinkInputFromJSON parses the native POST /rest/api/3/issueLink body.
// The payload may carry its own inwardIssue.key; a positional KEY is also
// accepted, and setting both to different values is a conflict — the CLI
// cannot know which one was meant. With no inwardIssue in the body, each
// positional key becomes the inward issue, so the body acts as a template
// for bulk linking.
func issueLinkInputFromJSON(jsonInput string, args []string) (issueLinkCreateInput, []string, error) {
	var in issueLinkCreateInput
	payload := map[string]any{}
	if err := cmdutil.ReadJSONFile(jsonInput, &payload); err != nil {
		return in, nil, err
	}
	// A typo'd top-level key would otherwise vanish silently — the body
	// has exactly four sections, so anything else is refused loudly.
	for k := range payload {
		switch k {
		case "type", "inwardIssue", "outwardIssue", "comment":
		default:
			return in, nil, fmt.Errorf("validation: issue link --json-input does not recognize %q; the native body carries type, inwardIssue, outwardIssue, and comment", k)
		}
	}
	switch v := payload["type"].(type) {
	case string:
		in.Type = strings.TrimSpace(v)
	case map[string]any:
		if name, ok := v["name"].(string); ok {
			in.Type = strings.TrimSpace(name)
		}
		if id, ok := v["id"].(string); ok {
			in.TypeID = strings.TrimSpace(id)
		}
	}
	if xstrings.AllEmpty(in.Type, in.TypeID) {
		return in, nil, fmt.Errorf("validation: issue link --json-input needs a type name or id, matching the Jira REST issueLink body")
	}
	in.To = issueLinkEndpointKey(payload["outwardIssue"])
	if in.To == "" {
		return in, nil, fmt.Errorf("validation: issue link --json-input needs an outwardIssue key, matching the Jira REST issueLink body")
	}
	if raw, ok := payload["comment"]; ok {
		comment, isMap := raw.(map[string]any)
		if !isMap {
			return in, nil, fmt.Errorf("validation: issue link --json-input comment must be an object with an ADF body")
		}
		// The comment body is rich text: parse it as ADF now so a
		// malformed document fails locally instead of as an opaque 400.
		if bodyRaw, hasBody := comment["body"]; hasBody {
			encoded, merr := json.Marshal(bodyRaw)
			if merr != nil {
				return in, nil, merr
			}
			if _, _, perr := adf.Parse(encoded); perr != nil {
				return in, nil, fmt.Errorf("issue link --json-input comment body: %w", perr)
			}
		}
		in.Comment = comment
	}
	inward := issueLinkEndpointKey(payload["inwardIssue"])
	keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
	if err != nil {
		return in, nil, err
	}
	switch {
	case inward == "" && len(keys) == 0:
		return in, nil, fmt.Errorf("validation: issue link needs an inward issue — pass a KEY argument or an inwardIssue key in the payload")
	case inward != "" && len(keys) > 0 && (len(keys) != 1 || !strings.EqualFold(keys[0], inward)):
		return in, nil, fmt.Errorf("validation: the inward issue is set twice — issue keys on the command line and inwardIssue %s in the payload; supply it once or align the values", inward)
	case len(keys) == 0:
		keys = []string{inward}
	}
	return in, keys, nil
}

// issueLinkEndpointKey reads a native link endpoint: {"key": "PROJ-1"} or a
// bare issue-key string.
func issueLinkEndpointKey(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if key, ok := v["key"].(string); ok {
			return strings.TrimSpace(key)
		}
	}
	return ""
}

func runIssueLinkCreateMany(cmd *cobra.Command, keys []string, parallelism int, in issueLinkCreateInput) error {
	if in.DryRun {
		in.previewType = cachedLinkTypeForPreview(cmd, in)
		results := xslices.Map(keys, func(key string) cmdutil.KeyResult[envelope.IssueLinkCreateOutput] {
			return cmdutil.KeyResult[envelope.IssueLinkCreateOutput]{Key: key, Value: issueLinkCreateData(key, in, true)}
		})
		return cmdutil.WriteKeyedResultsEnvelope(cmd, in.Command, results, func(_ string, data envelope.IssueLinkCreateOutput) any { return data })
	}
	client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for %s", in.Command)
	}
	in.previewType = liveLinkTypeForPreview(cmd, client, profile, in)
	service := cmdutil.ServicesForClient(client).IssueLink()
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (envelope.IssueLinkCreateOutput, error) {
		if _, err := service.Create(ctx, issueLinkRequestFor(key, in)); err != nil {
			return envelope.IssueLinkCreateOutput{}, err
		}
		return issueLinkCreateData(key, in, false), nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, in.Command, results, func(_ string, data envelope.IssueLinkCreateOutput) any { return data })
}

func issueLinkCreateData(key string, in issueLinkCreateInput, dryRun bool) envelope.IssueLinkCreateOutput {
	data := envelope.IssueLinkCreateOutput{
		InwardIssue:  cmdutil.IssueRef{Key: key},
		OutwardIssue: cmdutil.IssueRef{Key: in.To},
		Type:         in.Type,
		DryRun:       dryRun,
	}
	if in.TypeID != "" {
		data.TypeID = in.TypeID
	}
	if len(in.Comment) > 0 {
		data.Comment = in.Comment
	}
	if in.previewType != nil {
		inwardSentence, outwardSentence := in.previewType.PreviewSentences(key, in.To)
		data.Preview = &envelope.IssueLinkPreview{
			InwardIssueSentence:  inwardSentence,
			OutwardIssueSentence: outwardSentence,
		}
	}
	return data
}

// cachedLinkTypeForPreview resolves the link type's phrase pair from the
// per-profile linktypes cache at any age — the offline lookup a --dry-run
// is allowed. Phrase pairs change rarely enough that staleness is fine.
// nil (unprimed cache, unknown type, unresolvable profile) means no
// preview; the create itself is unaffected.
func cachedLinkTypeForPreview(cmd *cobra.Command, in issueLinkCreateInput) *jira.IssueLinkType {
	profile, err := cmdutil.ProfileForCommand(cmd)
	if err != nil {
		return nil
	}
	entry, ok, err := cache.ReadCachedOrEmpty(cmdutil.CacheKeyForProfile(cmd, profile), "linktypes")
	if !ok || err != nil {
		return nil
	}
	var types []jira.IssueLinkType
	if json.Unmarshal(entry.Data, &types) != nil {
		return nil
	}
	return matchLinkType(types, in.Type, in.TypeID)
}

// liveLinkTypeForPreview resolves the phrase pair on the live path: the
// cache first, then one /issueLinkType fetch that also primes the cache.
// Preview resolution is best-effort — a fetch failure degrades to no
// preview rather than blocking the create.
func liveLinkTypeForPreview(cmd *cobra.Command, client *jira.Client, profile config.Profile, in issueLinkCreateInput) *jira.IssueLinkType {
	if t := cachedLinkTypeForPreview(cmd, in); t != nil {
		return t
	}
	ttl := time.Duration(cachereg.TTLMinutesFor("linktypes")) * time.Minute
	data, _, _, _, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "linktypes", ttl, false, func() (json.RawMessage, error) {
		return fetchLinkTypesForCache(cmd, client)
	})
	if err != nil {
		return nil
	}
	var types []jira.IssueLinkType
	if json.Unmarshal(data, &types) != nil {
		return nil
	}
	return matchLinkType(types, in.Type, in.TypeID)
}

// matchLinkType finds the requested type by id (exact) or name
// (case-insensitive, matching Jira's own type-name handling).
func matchLinkType(types []jira.IssueLinkType, name, id string) *jira.IssueLinkType {
	for i := range types {
		if (id != "" && types[i].ID == id) || (name != "" && strings.EqualFold(types[i].Name, name)) {
			return &types[i]
		}
	}
	return nil
}

// issueLinkListCommand wires `jira issue link list KEY`.
//
// Derives from `GET /issue/{key}?fields=issuelinks`; flattens the
// inward/outward fork into a single direction-aware array; sorts by
// (direction, type.name, other_issue.key) ASC.
//
// `--raw` returns Atlassian's `issuelinks` array verbatim.
func issueLinkListCommand() *cobra.Command {
	var parallelism int
	cmd := &cobra.Command{
		Use:   "list KEY...",
		Short: "List issue links",
		Long: "List inward and outward links for one or more issues. Use it to inspect " +
			"dependencies before adding or deleting links.\n\n" +
			"The command flattens Jira's inward/outward link shape into a direction-aware " +
			"array. Multiple issue keys are fetched with bounded parallelism.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Example: `# List every link on an issue
$ jira issue link list PROJ-123

# List links for several issues as JSON
$ jira issue link list PROJ-123 PROJ-124 --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link.list")
			}
			service := cmdutil.ServicesForClient(client).IssueLink()
			if len(keys) == 1 {
				var links []jira.IssueLinkView
				if err := cmdutil.Spin(cmd, "issue.link.list", func(ctx context.Context) error {
					var e error
					links, _, e = service.List(ctx, keys[0])
					return e
				}); err != nil {
					return err
				}
				return cmdutil.WriteEnvelope(cmd, "issue.link.list", issueLinkListData(keys[0], links))
			}
			results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) ([]jira.IssueLinkView, error) {
				links, _, err := service.List(ctx, key)
				return links, err
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.link.list", results, func(key string, links []jira.IssueLinkView) any {
				return issueLinkListData(key, links)
			})
		},
	}
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func issueLinkListData(key string, links []jira.IssueLinkView) envelope.IssueLinkListOutput {
	return envelope.IssueLinkListOutput{
		Issue: cmdutil.IssueRef{Key: key},
		Links: links,
		Count: len(links),
	}
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
		Use:   "delete KEY LINK_ID",
		Short: "Remove an issue link by id",
		Long: "Delete an issue link by its global link ID. Use `issue link list` first when " +
			"you need to find the ID for an issue.\n\n" +
			"`KEY` is accepted for completion and context, but Jira deletes links by global " +
			"ID. `--dry-run` previews the deletion and never contacts Jira. Live deletes " +
			"require `--force` in headless, agent, or `--no-input` mode.",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"clib": "dynamic-args='issuekey'"},
		Example: `# Preview deleting a link
$ jira issue link delete PROJ-123 10001 --dry-run

# Remove a link by its global id without a prompt
$ jira issue link delete PROJ-123 10001 --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			key, linkID := args[0], args[1]
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "issue.link.delete", envelope.IssueLinkDeleteOutput{
					Issue:  cmdutil.IssueRef{Key: key},
					LinkID: linkID,
					DryRun: true,
				})
			}
			det := cmdutil.DetectorFromContext(cmd)
			if !force {
				if !det.IsTTY || det.Agent || noInput {
					return cli.NewCLIInputError(cli.InputForceRequired, "issue link delete requires --force in headless / agent / --no-input mode")
				}
				if ok, err := confirmDestructive(cmd, "link delete", linkID); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "link delete", nil)
				}
			}
			client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.link.delete")
			}
			var resp *jira.Response
			if err := cmdutil.Spin(cmd, "issue.link.delete", func(ctx context.Context) error {
				var e error
				resp, e = cmdutil.ServicesForClient(client).IssueLink().Delete(ctx, linkID)
				return e
			}); err != nil {
				return err
			}
			// data.link_id MUST echo the supplied id verbatim
			// regardless of the source KEY — links are global; the
			// CLI is explicit about which id was removed.
			return cmdutil.WriteEnvelopeWithResponse(cmd, "issue.link.delete", envelope.IssueLinkDeleteOutput{
				Issue:   cmdutil.IssueRef{Key: key},
				LinkID:  linkID,
				Deleted: true,
				DryRun:  false,
			}, resp)
		},
	}
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm destructive removal (required under `--no-input`)")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without removing the link")
	return cmd
}

// issueLinkTypesCommand wires `jira issue link types`.
//
// Reads from the `linktypes` cache when fresh; primes via
// /issueLinkType when stale or `--refresh` is supplied. The default TTL
// tracks the linktypes cache resource, so this command and
// `jira cache linktypes` agree on freshness.
//
// `--raw` returns Atlassian's native `{issueLinkTypes: [...]}`
// envelope verbatim.
func issueLinkTypesCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "types",
		Short: "List configured issue link types",
		Long: "List the issue link types configured in the active Jira site. Use it before " +
			"creating links when you need the exact `--type` value and direction labels.\n\n" +
			"The command reads the `linktypes` cache when fresh. On a cache miss, stale " +
			"entry, or `--refresh`, it fetches Jira's issue-link type metadata and updates " +
			"the per-profile cache.",
		Args: cobra.NoArgs,
		Example: `# Show the configured link types
$ jira issue link types

# Force a refresh past the cache
$ jira issue link types --refresh

# Return link types as JSON
$ jira issue link types --output=json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "linktypes", ttl, refresh, func() (json.RawMessage, error) {
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
			envelopeData := envelope.IssueLinkTypesOutput{
				LinkTypes:        types,
				Count:            len(types),
				FromCache:        fromCache,
				FetchedAt:        fetchedAt.UTC().Format(time.RFC3339),
				CacheState:       cmdutil.CacheStateForCount(cacheSourceState, len(types)),
				CacheSourceState: cacheSourceState,
				CacheEmpty:       len(types) == 0,
			}
			return cmdutil.WriteEnvelope(cmd, "issue.link.types", envelopeData)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &refresh, "refresh", false, "Force a fetch even when the cache is fresh", clib.FlagExtra{Group: "Cache"})
	cmdutil.AddIntVar(cmd.Flags(), &ttlMinutes, "ttl-minutes", cachereg.TTLMinutesFor("linktypes"), "Freshness window before automatic refresh", clib.FlagExtra{Group: "Cache", Placeholder: "N"})
	return cmd
}

// fetchLinkTypesForCache calls /issueLinkType, unwraps the payload,
// and returns the JSON-encoded slice. Reused by the cache primer
// command in cache.go.
func fetchLinkTypesForCache(cmd *cobra.Command, client *jira.Client) (json.RawMessage, error) {
	var types []jira.IssueLinkType
	if err := cmdutil.Spin(cmd, "issue.link.types", func(ctx context.Context) error {
		var e error
		types, _, e = cmdutil.ServicesForClient(client).IssueLinkType().List(ctx)
		return e
	}); err != nil {
		return nil, err
	}
	return cmdutil.MarshalNonNilSlice(types)
}
