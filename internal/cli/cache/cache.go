package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

var cacheClearResources = []string{"labels", "projects", "epics", "fields", "issuetypes", "linktypes", "boards", "statuses", "priorities"}

// NewCommand groups per-resource cache primers + housekeeping. Each
// subcommand fetches the resource, writes the JSON-encoded list under a
// config/site/profile cache namespace, and emits the list as the envelope's
// data so agents (and completion functions) can pipe it.
//
// Reads are cheap (single file) — see `internal/cache` for the format.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("cache", "Prime / inspect the local Jira metadata cache", "agent")
	cmd.AddCommand(cacheLabelsCommand())
	cmd.AddCommand(cacheProjectsCommand())
	cmd.AddCommand(cacheEpicsCommand())
	cmd.AddCommand(cacheFieldsCommand())
	cmd.AddCommand(cacheIssueTypesCommand())
	cmd.AddCommand(cacheLinkTypesCommand())
	cmd.AddCommand(cacheBoardsCommand())
	cmd.AddCommand(cacheStatusesCommand())
	cmd.AddCommand(cachePrioritiesCommand())
	cmd.AddCommand(cacheClearCommand())
	return cmd
}

func cacheLabelsCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "Cache and print the global Jira label list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "labels", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.labels")
				}
				labels, _, err := cmdutil.ServicesForClient(client).Label().List(cmd.Context(), nil)
				if err != nil {
					return nil, err
				}
				return cmdutil.MarshalNonNilSlice(labels)
			})
			if err != nil {
				return err
			}
			var labels []string
			if err := json.Unmarshal(data, &labels); err != nil {
				return fmt.Errorf("cache.labels: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"labels":     labels,
				"count":      len(labels),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(labels))
			return cmdutil.WriteEnvelope(cmd, "cache.labels", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

func cacheProjectsCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Cache and print the visible Jira project list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "projects", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.projects")
				}
				projects, _, err := cmdutil.ServicesForClient(client).Project(0).List(cmd.Context(), nil)
				if err != nil {
					return nil, err
				}
				return cmdutil.MarshalNonNilSlice(projects)
			})
			if err != nil {
				return err
			}
			var projects []jira.ProjectSummary
			if err := json.Unmarshal(data, &projects); err != nil {
				return fmt.Errorf("cache.projects: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"projects":   projects,
				"count":      len(projects),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(projects))
			return cmdutil.WriteEnvelope(cmd, "cache.projects", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

// cacheEpic is the agent-friendly subset stored on disk. Distinct from
// jira.Epic to avoid the shape-fragility risk of caching internal types.
type cacheEpic struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func cacheEpicsCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "Cache and print the visible epic list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "epics", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.epics")
				}
				return fetchEpicsForCache(cmd.Context(), client)
			})
			if err != nil {
				return err
			}
			var epics []cacheEpic
			if err := json.Unmarshal(data, &epics); err != nil {
				return fmt.Errorf("cache.epics: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"epics":      epics,
				"count":      len(epics),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(epics))
			return cmdutil.WriteEnvelope(cmd, "cache.epics", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

func fetchEpicsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	issues, _, err := cmdutil.ServicesForClient(client).Epic().List(ctx, &jira.ListOptions{MaxResults: 200})
	if err != nil {
		return nil, err
	}
	out := make([]cacheEpic, 0, len(issues))
	for _, iss := range issues {
		e := cacheEpic{}
		if iss != nil && iss.Key != nil {
			e.Key = *iss.Key
		}
		if iss != nil && iss.Fields != nil {
			if iss.Fields.Summary != nil {
				e.Summary = *iss.Fields.Summary
			}
			if iss.Fields.Status != nil && iss.Fields.Status.Name != nil {
				e.Status = *iss.Fields.Status.Name
			}
		}
		out = append(out, e)
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheField is the agent-friendly subset of a Jira field. ID is the
// stable customfield_xxxxx identifier; name is the display label.
type cacheField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func cacheFieldsCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "fields",
		Short: "Cache and print the visible Jira field list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "fields", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.fields")
				}
				return fetchFieldsForCache(cmd.Context(), client)
			})
			if err != nil {
				return err
			}
			var fields []cacheField
			if err := json.Unmarshal(data, &fields); err != nil {
				return fmt.Errorf("cache.fields: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"fields":     fields,
				"count":      len(fields),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(fields))
			return cmdutil.WriteEnvelope(cmd, "cache.fields", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

func fetchFieldsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", "rest/api/3/field", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Schema struct {
			Type string `json:"type"`
		} `json:"schema"`
	}
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheField, 0, len(raw))
	for _, f := range raw {
		out = append(out, cacheField{ID: f.ID, Name: f.Name, Type: f.Schema.Type})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheIssueType is the cached subset of a project's issue-type schema.
type cacheIssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

func cacheIssueTypesCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "issuetypes",
		Short: "Cache and print the visible Jira issue-type list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "issuetypes", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.issuetypes")
				}
				return fetchIssueTypesForCache(cmd.Context(), client)
			})
			if err != nil {
				return err
			}
			var types []cacheIssueType
			if err := json.Unmarshal(data, &types); err != nil {
				return fmt.Errorf("cache.issuetypes: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"issuetypes": types,
				"count":      len(types),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(types))
			return cmdutil.WriteEnvelope(cmd, "cache.issuetypes", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

func fetchIssueTypesForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", "rest/api/3/issuetype", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Subtask bool   `json:"subtask"`
	}
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheIssueType, 0, len(raw))
	for _, t := range raw {
		out = append(out, cacheIssueType{ID: t.ID, Name: t.Name, Subtask: t.Subtask})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheNamedValue is the cached subset of a workflow status or priority:
// the name completion offers and the id for the shell tooltip.
type cacheNamedValue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func cacheStatusesCommand() *cobra.Command {
	return cacheNamedValueCommand("statuses", "Cache and print the visible Jira status list", "rest/api/3/status")
}

func cachePrioritiesCommand() *cobra.Command {
	return cacheNamedValueCommand("priorities", "Cache and print the visible Jira priority list", "rest/api/3/priority")
}

// cacheNamedValueCommand builds a cache subcommand for a flat {id,name} Jira
// metadata list (statuses, priorities). Both endpoints return an unpaginated
// array, so one GET fills the cache that completion reads for --status and
// --priority. Default TTL 60 minutes.
func cacheNamedValueCommand(resource, short, path string) *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   resource,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), resource, time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.%s", resource)
				}
				return fetchNamedValuesForCache(cmd.Context(), client, path)
			})
			if err != nil {
				return err
			}
			var values []cacheNamedValue
			if err := json.Unmarshal(data, &values); err != nil {
				return fmt.Errorf("cache.%s: decode cached payload: %w", resource, err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				resource:     values,
				"count":      len(values),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(values))
			return cmdutil.WriteEnvelope(cmd, "cache."+resource, envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

func fetchNamedValuesForCache(ctx context.Context, client *jira.Client, path string) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var raw []cacheNamedValue
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheNamedValue, 0, len(raw))
	for _, v := range raw {
		if v.Name == "" {
			continue
		}
		out = append(out, cacheNamedValue{ID: v.ID, Name: v.Name})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheLinkTypesCommand primes the per-profile cache of issue-link
// types so completion and the `jira issue link types` command can
// resolve names without round-tripping. Default TTL 60 minutes.
func cacheLinkTypesCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "linktypes",
		Short: "Cache and print the configured issue-link types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, cacheSourceState, err := cmdutil.CacheReadOrFetch(cmdutil.CacheKeyForProfile(cmd, profile), "linktypes", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.linktypes")
				}
				types, _, err := cmdutil.ServicesForClient(client).IssueLinkType().List(cmd.Context())
				if err != nil {
					return nil, err
				}
				return cmdutil.MarshalNonNilSlice(types)
			})
			if err != nil {
				return err
			}
			var types []jira.IssueLinkType
			if err := json.Unmarshal(data, &types); err != nil {
				return fmt.Errorf("cache.linktypes: decode cached payload: %w", err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				"link_types": types,
				"count":      len(types),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(types))
			return cmdutil.WriteEnvelope(cmd, "cache.linktypes", envelopeData)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	return cmd
}

// cacheBoardsCommand primes the per-profile boards cache so completion
// and the `--board` resolver can serve names without hitting the wire.
// Default TTL 60 minutes. Mirrors cacheLinkTypesCommand's flag surface;
// rendering differs because the envelope carries truncation + per-board
// project metadata.
func cacheBoardsCommand() *cobra.Command {
	var refresh, unbounded bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "boards",
		Short: "Cache and print the visible Jira agile boards",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute

			cacheSourceState := cmdutil.CacheStateRefresh

			// Cache hit path — short-circuit before fetching unless --refresh.
			if !refresh {
				entry, present, stale, readErr := cache.Read(cmdutil.CacheKeyForProfile(cmd, profile), "boards", ttl)
				switch {
				case readErr != nil:
					cacheSourceState = cmdutil.CacheStateMalformed
				case present && !stale:
					return emitCachedBoardsEnvelope(cmd, profile.Name, entry, true, cmdutil.CacheStateFresh)
				case present && stale:
					cacheSourceState = cmdutil.CacheStateStale
				default:
					cacheSourceState = cmdutil.CacheStateMissing
				}
			}

			if !ok {
				return fmt.Errorf("jira base URL is required for cache.boards")
			}

			file, entry, warnings, err := cmdutil.PrimeAndCacheBoards(cmd.Context(), cmdutil.CacheKeyForProfile(cmd, profile), client, ttlMinutes, unbounded)
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":          profile.Name,
				"primed":           true,
				"from_cache":       false,
				"boards_count":     len(file.Items),
				"fetched_at":       entry.FetchedAt.UTC().Format(time.RFC3339),
				"ttl_seconds":      file.TTLSeconds,
				"truncated":        file.Truncated,
				"truncated_reason": file.TruncatedReason,
			}
			cmdutil.AddCacheStateFields(data, cacheSourceState, len(file.Items))
			return cmdutil.WriteEnvelopeWithRawWarnings(cmd, "cache.boards", data, warnings)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmd.Flags().BoolVar(&unbounded, "unbounded", false, "Walk every page (disables the default 100-page / 10 000-board cap)")
	cmdutil.ExtendRefreshFlags(cmd.Flags())
	// No --dry-run: the cache primer's whole purpose is a live fetch
	// plus a cache write, so a "dry-run" flag here could not be honest.
	return cmd
}

// emitCachedBoardsEnvelope renders the cache-hit envelope shape
// (primed=false, from_cache=true). The truncated marker is preserved
// from the cached file so consumers can detect a stale partial-prime.
func emitCachedBoardsEnvelope(cmd *cobra.Command, profileName string, entry cache.Entry, fromCache bool, cacheSourceState string) error {
	var file jira.BoardsCacheFile
	if err := json.Unmarshal(entry.Data, &file); err != nil {
		// Fallback shape: bare boards array (legacy resolver fixtures).
		var arr []jira.Board
		if err2 := json.Unmarshal(entry.Data, &arr); err2 != nil {
			return fmt.Errorf("cache.boards: decode cache: %w", err)
		}
		file.Items = arr
	}
	var warnings []map[string]any
	if file.Truncated {
		// Surface the truncation marker again so cached re-reads see it
		// (otherwise the warning silently disappears between primes).
		switch file.TruncatedReason {
		case "max_pages":
			warnings = append(warnings, map[string]any{
				"type": "cache-truncated", "resource": "boards",
				"reason": "max_pages", "limit": 100,
				"remediation": "Re-run with --unbounded if you need every board.",
			})
		case "max_results":
			warnings = append(warnings, map[string]any{
				"type": "cache-truncated", "resource": "boards",
				"reason": "max_results", "limit": 10_000,
				"remediation": "Re-run with --unbounded if you need every board.",
			})
		case "rate_limit":
			warnings = append(warnings, map[string]any{
				"type": "rate-limit-during-paginate", "resource": "boards",
				"pages_fetched":       file.PagesFetched,
				"retry_after_seconds": file.RetryAfterSeconds,
				"remediation":         "Re-run `jira cache boards --refresh` after the rate-limit window resets.",
			})
		}
	}
	data := map[string]any{
		"profile":          profileName,
		"primed":           false,
		"from_cache":       fromCache,
		"boards_count":     len(file.Items),
		"fetched_at":       entry.FetchedAt.UTC().Format(time.RFC3339),
		"ttl_seconds":      file.TTLSeconds,
		"truncated":        file.Truncated,
		"truncated_reason": file.TruncatedReason,
	}
	cmdutil.AddCacheStateFields(data, cacheSourceState, len(file.Items))
	return cmdutil.WriteEnvelopeWithRawWarnings(cmd, "cache.boards", data, warnings)
}

func cacheClearCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "clear [resource]",
		Short:       "Remove cached data (a single resource, or the whole profile)",
		Long:        `With no argument, removes every cache file under the active profile. With a resource name (labels, projects, epics, …), removes just that file.`,
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"clib": "dynamic-args='cacheresource'"},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return cacheClearResources, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && !isCacheClearResource(args[0]) {
				return cli.NewCLIInputError(
					cli.InputArgValueInvalid,
					fmt.Sprintf("unknown cache resource %q; valid resources: %s", args[0], strings.Join(cacheClearResources, ", ")),
				)
			}
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
			if err != nil {
				return err
			}
			if len(args) == 0 {
				n, err := cache.ClearProfile(cmdutil.CacheKeyForProfile(cmd, profile))
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelope(cmd, "cache.clear", map[string]any{
					"profile": profile.Name,
					"removed": n,
				})
			}
			ok, err := cache.Clear(cmdutil.CacheKeyForProfile(cmd, profile), args[0])
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "cache.clear", map[string]any{
				"profile":  profile.Name,
				"resource": args[0],
				"removed":  ok,
			})
		},
	}
	return cmd
}

func isCacheClearResource(resource string) bool {
	return slices.Contains(cacheClearResources, resource)
}
