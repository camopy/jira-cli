package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// cacheCommand groups per-resource cache primers + housekeeping. Each
// subcommand fetches the resource, writes the JSON-encoded list under
// ~/.cache/jira-cli/<profile>/<resource>.json, and emits the list as the
// envelope's data so agents (and completion functions) can pipe it.
//
// Reads are cheap (single file) — see `internal/cache` for the format.
func cacheCommand() *cobra.Command {
	cmd := groupCommand("cache", "Prime / inspect the local Jira metadata cache", "agent")
	cmd.AddCommand(cacheLabelsCommand())
	cmd.AddCommand(cacheProjectsCommand())
	cmd.AddCommand(cacheEpicsCommand())
	cmd.AddCommand(cacheFieldsCommand())
	cmd.AddCommand(cacheIssueTypesCommand())
	cmd.AddCommand(cacheLinkTypesCommand())
	cmd.AddCommand(cacheBoardsCommand())
	cmd.AddCommand(cacheClearCommand())
	return cmd
}

// cacheReadOrFetch is the read-then-fetch helper every subcommand uses.
// Returns the JSON-encoded resource bytes plus a flag telling the caller
// whether the value was served from disk.
func cacheReadOrFetch(profile, resource string, ttl time.Duration, refresh bool, fetch func() (json.RawMessage, error)) (json.RawMessage, bool, time.Time, error) {
	if !refresh {
		if entry, ok, stale, err := cache.Read(profile, resource, ttl); err == nil && ok && !stale {
			return entry.Data, true, entry.FetchedAt, nil
		}
	}
	data, err := fetch()
	if err != nil {
		return nil, false, time.Time{}, err
	}
	entry, err := cache.Write(profile, resource, data)
	if err != nil {
		return nil, false, time.Time{}, err
	}
	return entry.Data, false, entry.FetchedAt, nil
}

// marshalNonNilSlice marshals `v` but rewrites nil slices to `[]` so cache
// files never contain `null`. Without this, decoding back into a typed
// slice produces nil and downstream consumers either crash or paper over
// the bug with `if x == nil` patches.
func marshalNonNilSlice(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return json.RawMessage("[]"), nil
	}
	return b, nil
}

func cacheLabelsCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "Cache and print the global Jira label list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "labels", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.labels")
				}
				labels, _, err := jira.NewLabelService(client).List(cmd.Context(), nil)
				if err != nil {
					return nil, err
				}
				return marshalNonNilSlice(labels)
			})
			if err != nil {
				return err
			}
			var labels []string
			if err := json.Unmarshal(data, &labels); err != nil {
				return fmt.Errorf("cache.labels: decode cached payload: %w", err)
			}
			return writeEnvelope(cmd, "cache.labels", map[string]any{
				"profile":    profile.Name,
				"labels":     labels,
				"count":      len(labels),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "projects", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.projects")
				}
				projects, _, err := jira.NewProjectService(client, 0).List(cmd.Context(), nil)
				if err != nil {
					return nil, err
				}
				return marshalNonNilSlice(projects)
			})
			if err != nil {
				return err
			}
			var projects []jira.ProjectSummary
			if err := json.Unmarshal(data, &projects); err != nil {
				return fmt.Errorf("cache.projects: decode cached payload: %w", err)
			}
			return writeEnvelope(cmd, "cache.projects", map[string]any{
				"profile":    profile.Name,
				"projects":   projects,
				"count":      len(projects),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "epics", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
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
			return writeEnvelope(cmd, "cache.epics", map[string]any{
				"profile":    profile.Name,
				"epics":      epics,
				"count":      len(epics),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	return cmd
}

func fetchEpicsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	issues, _, err := jira.NewEpicService(client).List(ctx, &jira.ListOptions{MaxResults: 200})
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
	return marshalNonNilSlice(out)
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "fields", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
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
			return writeEnvelope(cmd, "cache.fields", map[string]any{
				"profile":    profile.Name,
				"fields":     fields,
				"count":      len(fields),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
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
	return marshalNonNilSlice(out)
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "issuetypes", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
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
			return writeEnvelope(cmd, "cache.issuetypes", map[string]any{
				"profile":    profile.Name,
				"issuetypes": types,
				"count":      len(types),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
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
	return marshalNonNilSlice(out)
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			data, fromCache, fetchedAt, err := cacheReadOrFetch(profile.Name, "linktypes", time.Duration(ttlMinutes)*time.Minute, refresh, func() (json.RawMessage, error) {
				if !ok {
					return nil, fmt.Errorf("jira base URL is required for cache.linktypes")
				}
				types, _, err := jira.NewIssueLinkTypeService(client).List(cmd.Context())
				if err != nil {
					return nil, err
				}
				return marshalNonNilSlice(types)
			})
			if err != nil {
				return err
			}
			var types []jira.IssueLinkType
			if err := json.Unmarshal(data, &types); err != nil {
				return fmt.Errorf("cache.linktypes: decode cached payload: %w", err)
			}
			return writeEnvelope(cmd, "cache.linktypes", map[string]any{
				"profile":    profile.Name,
				"link_types": types,
				"count":      len(types),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
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
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute

			// Cache hit path — short-circuit before fetching unless --refresh.
			if !refresh {
				if entry, present, stale, readErr := cache.Read(profile.Name, "boards", ttl); readErr == nil && present && !stale {
					return emitCachedBoardsEnvelope(cmd, profile.Name, entry, true)
				}
			}

			if !ok {
				return fmt.Errorf("jira base URL is required for cache.boards")
			}

			file, warnings, err := primeBoards(cmd.Context(), client, ttlMinutes, unbounded)
			if err != nil {
				return err
			}

			body, err := json.Marshal(file)
			if err != nil {
				return fmt.Errorf("cache.boards: marshal cache: %w", err)
			}
			entry, err := cache.Write(profile.Name, "boards", body)
			if err != nil {
				return fmt.Errorf("cache.boards: write: %w", err)
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
			return writeEnvelopeWithRawWarnings(cmd, "cache.boards", data, warnings)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a fetch even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmd.Flags().BoolVar(&unbounded, "unbounded", false, "Walk every page (disables the default 100-page / 10 000-board cap)")
	// Cache primer is a "read"-class command; accept --dry-run as a
	// no-op so callers can pass uniform flag sets through pipelines.
	var dryRun bool
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "No-op on the cache primer; accepted for flag-set consistency")
	return cmd
}

// primeBoards drains /rest/agile/1.0/board, populates per-board project
// keys via /board/{id}/project, and assembles a BoardsCacheFile ready
// for cache.Write. Surfaces truncation as both file markers and
// envelope warnings.
func primeBoards(ctx context.Context, client *jira.Client, ttlMinutes int, unbounded bool) (jira.BoardsCacheFile, []map[string]any, error) {
	svc := jira.NewBoardService(client)
	res, err := svc.ListAll(ctx, jira.BoardDrainOptions{Unbounded: unbounded})
	if err != nil {
		return jira.BoardsCacheFile{}, nil, err
	}

	// Materialize the wire records (deref pointers, normalize names) so
	// SanitizeBoardsForCache can apply the data-model invariants
	// (ID > 0, trimmed Name non-empty) on a uniform slice.
	materialized := make([]jira.Board, 0, len(res.Boards))
	for _, b := range res.Boards {
		if b == nil {
			continue
		}
		clean := *b
		if clean.Name != nil {
			n := jira.NormalizeBoardName(*clean.Name)
			clean.Name = &n
		}
		materialized = append(materialized, clean)
	}
	items, dropped := jira.SanitizeBoardsForCache(materialized)

	// Per-board projects — only after sanitize, so we don't burn an
	// HTTP roundtrip on a record we're about to drop. Keys are then
	// filtered for JQL-meta characters; SanitizeBoardsForCache invariants
	// already proved ID is set, so the dereference below is safe.
	var droppedKeys int
	for i := range items {
		keys, _, perr := svc.ProjectsForBoard(ctx, *items[i].ID)
		if perr == nil {
			cleanKeys, dk := jira.SanitizeProjectKeys(keys)
			droppedKeys += dk
			items[i].ProjectKeys = cleanKeys
		}
		if items[i].ProjectKeys == nil {
			items[i].ProjectKeys = []string{}
		}
	}

	ttl := ttlMinutes * 60
	file := jira.BoardsCacheFile{
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
		TTLSeconds:      ttl,
		Truncated:       res.Truncated,
		TruncatedReason: res.TruncatedReason,
		Items:           items,
	}

	var warnings []map[string]any
	switch res.TruncatedReason {
	case "max_pages", "max_results":
		limit := 100
		if res.TruncatedReason == "max_results" {
			limit = 10_000
		}
		warnings = append(warnings, map[string]any{
			"type":        "cache-truncated",
			"resource":    "boards",
			"reason":      res.TruncatedReason,
			"limit":       limit,
			"remediation": "Re-run with --unbounded if you need every board.",
		})
	case "rate_limit":
		warnings = append(warnings, map[string]any{
			"type":        "rate-limit-during-paginate",
			"resource":    "boards",
			"remediation": "Re-run `jira cache boards --refresh` after the rate-limit window resets.",
		})
	}
	if dropped > 0 {
		warnings = append(warnings, map[string]any{
			"type":     "bad-records-dropped",
			"resource": "boards",
			"count":    dropped,
			"reason":   "missing or invalid id/name",
		})
	}
	if droppedKeys > 0 {
		warnings = append(warnings, map[string]any{
			"type":     "bad-project-keys-dropped",
			"resource": "boards",
			"count":    droppedKeys,
			"reason":   "key contains JQL meta-characters",
		})
	}
	return file, warnings, nil
}

// emitCachedBoardsEnvelope renders the cache-hit envelope shape
// (primed=false, from_cache=true). The truncated marker is preserved
// from the cached file so consumers can detect a stale partial-prime.
func emitCachedBoardsEnvelope(cmd *cobra.Command, profileName string, entry cache.Entry, fromCache bool) error {
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
				"remediation": "Re-run `jira cache boards --refresh` after the rate-limit window resets.",
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
	return writeEnvelopeWithRawWarnings(cmd, "cache.boards", data, warnings)
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
			return []string{"labels", "projects", "epics", "fields", "issuetypes", "linktypes", "boards"}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, profile, _, _ := jiraClientForCommand(cmd)
			if len(args) == 0 {
				n, err := cache.ClearProfile(profile.Name)
				if err != nil {
					return err
				}
				return writeEnvelope(cmd, "cache.clear", map[string]any{
					"profile": profile.Name,
					"removed": n,
				})
			}
			ok, err := cache.Clear(profile.Name, args[0])
			if err != nil {
				return err
			}
			return writeEnvelope(cmd, "cache.clear", map[string]any{
				"profile":  profile.Name,
				"resource": args[0],
				"removed":  ok,
			})
		},
	}
	return cmd
}
