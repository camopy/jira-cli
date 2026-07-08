package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

// cacheClearResources is the set `cache clear` accepts. It derives from the
// resource registry so the list cannot drift from the primer subcommands.
var cacheClearResources = registry.ResourceNames()

// NewCommand groups per-resource cache primers + housekeeping. Every primer
// fetches its resource, writes the JSON-encoded list under a
// config/site/profile cache namespace, and emits the list as the envelope's
// data so agents (and completion functions) can pipe it. The subcommands are
// generated from the resource registry — the single source of truth — except
// boards, whose Fetch is nil because its envelope carries truncation and
// per-board project metadata the generic primer cannot express.
//
// Reads are cheap (single file) — see `internal/cache` for the format.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("cache", "Prime / inspect the local Jira metadata cache", "agent")
	cmd.Long = "Prime and inspect the local Jira metadata cache that powers fast listing and " +
		"shell completion. `jira cache refresh` refetches stale resources (or all with " +
		"`--force`), `jira cache clear` removes cached files, and `jira cache boards` shows " +
		"the cached boards.\n\n" +
		"The cache lives under a per-profile directory and is always safe to clear — the next " +
		"command re-primes what it needs. `--dry-run` previews any of these without touching " +
		"Jira or disk."
	cmd.Example = `$ jira cache refresh

# Refetch everything, ignoring freshness
$ jira cache refresh --force

# Remove cached metadata for the active profile
$ jira cache clear`
	for _, r := range registry.Registry {
		if r.Fetch == nil {
			cmd.AddCommand(cacheBoardsCommand())
			continue
		}
		cmd.AddCommand(newCachePrimerCommand(r))
	}
	cmd.AddCommand(cacheRefreshCommand())
	cmd.AddCommand(cacheClearCommand())
	return cmd
}

// newCachePrimerCommand builds the `cache <name>` primer for a registry
// resource. The read-then-fetch flow, envelope shape, and flag surface are
// identical across every flat-list resource (labels, projects, epics, fields,
// issuetypes, linktypes, statuses, priorities, resolutions); only the resource
// identity and its Fetch vary, both carried by the registry entry.
func newCachePrimerCommand(r registry.Resource) *cobra.Command {
	var refresh bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   r.Name,
		Short: r.Short,
		Long: "Cache and print one Jira metadata resource for the active profile. Use it " +
			"when completions, agents, or later commands need a local copy of `" + r.Name + "`.\n\n" +
			"The command serves a fresh cache entry when present. On a miss, stale entry, " +
			"or `--refresh`, it contacts Jira, writes the per-profile cache file, and " +
			"prints the cached list.",
		Example: `# Use the cache if fresh, otherwise fetch from Jira
$ jira cache ` + r.Name + `

# Force a re-fetch even when the cache is fresh
$ jira cache ` + r.Name + ` --refresh

# Return the cached list in a parseable shape
$ jira cache ` + r.Name + ` --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute
			cacheSourceState := cmdutil.CacheStateRefresh
			var (
				data      json.RawMessage
				fromCache bool
				fetchedAt time.Time
			)
			if !refresh {
				entry, present, stale, readErr := cache.Read(cmdutil.CacheKeyForProfile(cmd, profile), r.Name, ttl)
				switch {
				case readErr != nil:
					cacheSourceState = cmdutil.CacheStateMalformed
				case present && !stale:
					data, fromCache, fetchedAt, cacheSourceState = entry.Data, true, entry.FetchedAt, cmdutil.CacheStateFresh
				case present && stale:
					cacheSourceState = cmdutil.CacheStateStale
				default:
					cacheSourceState = cmdutil.CacheStateMissing
				}
			}
			if !fromCache {
				if !ok {
					return fmt.Errorf("jira base URL is required for cache.%s", r.Name)
				}
				err = cmdutil.Spin(cmd, "cache."+r.Name, func(ctx context.Context) error {
					var spinErr error
					data, spinErr = r.Fetch(ctx, client)
					return spinErr
				})
				if err != nil {
					return err
				}
				var entry cache.Entry
				entry, err = cache.Write(cmdutil.CacheKeyForProfile(cmd, profile), r.Name, data)
				if err != nil {
					return err
				}
				fromCache, fetchedAt = false, entry.FetchedAt
			}
			items, err := decodeCacheList(data)
			if err != nil {
				return fmt.Errorf("cache.%s: decode cached payload: %w", r.Name, err)
			}
			envelopeData := map[string]any{
				"profile":    profile.Name,
				r.Key():      items,
				"count":      len(items),
				"from_cache": fromCache,
				"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
			}
			cmdutil.AddCacheStateFields(envelopeData, cacheSourceState, len(items))
			return cmdutil.WriteEnvelope(cmd, "cache."+r.Name, envelopeData)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &refresh, "refresh", false, "Force a fetch even when the cache is fresh",
		clib.FlagExtra{Group: "Cache", Terse: "force fetch"})
	cmdutil.AddIntVar(cmd.Flags(), &ttlMinutes, "ttl-minutes", r.TTLMinutes, "Freshness window before automatic refresh",
		clib.FlagExtra{Group: "Cache", Placeholder: "N", Terse: "freshness window"})
	return cmd
}

// decodeCacheList splits a cached JSON array into its elements, each kept as
// raw bytes. The primer emits this []json.RawMessage rather than the bare
// json.RawMessage so the value is a slice of elements, not a slice of bytes:
// the JSON envelope still marshals each element verbatim, but the human/plain
// renderer — which summarizes a slice as "[N items]" by its reflect length —
// counts items, not bytes. len(items) is also the envelope count.
func decodeCacheList(data json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// cacheBoardsCommand primes the per-profile boards cache so completion
// and the `--board` resolver can serve names without hitting the wire.
// Mirrors the generic primer's flag surface; rendering differs because the
// envelope carries truncation + per-board project metadata.
func cacheBoardsCommand() *cobra.Command {
	var refresh, unbounded bool
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "boards",
		Short: "Cache and print the visible Jira agile boards",
		Long: "Prime the per-profile boards cache and print its status. Use it before " +
			"board-name completion, board-scoped issue listing, or any workflow that " +
			"needs board IDs without repeated agile API calls.\n\n" +
			"The command serves a fresh cache entry unless `--refresh` is set. Large " +
			"sites are capped by default; use `--unbounded` when you need every board and " +
			"are prepared for a longer live read.",
		Args: cobra.NoArgs,
		Example: `# Use the boards cache if fresh, otherwise fetch from Jira
$ jira cache boards

# Force a re-fetch even when the cache is still fresh
$ jira cache boards --refresh

# Walk every page, ignoring the default 100-page cap
$ jira cache boards --unbounded`,
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

			var (
				file     jira.BoardsCacheFile
				entry    cache.Entry
				warnings []map[string]any
			)
			err = cmdutil.Spin(cmd, "cache.boards", func(ctx context.Context) error {
				var spinErr error
				file, entry, warnings, spinErr = cmdutil.PrimeAndCacheBoards(ctx, cmdutil.CacheKeyForProfile(cmd, profile), client, ttlMinutes, unbounded)
				return spinErr
			})
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
	cmdutil.AddBoolVar(cmd.Flags(), &refresh, "refresh", false, "Force a fetch even when the cache is fresh",
		clib.FlagExtra{Group: "Cache", Terse: "force fetch"})
	cmdutil.AddIntVar(cmd.Flags(), &ttlMinutes, "ttl-minutes", registry.TTLMinutesFor("boards"), "Freshness window before automatic refresh",
		clib.FlagExtra{Group: "Cache", Placeholder: "N", Terse: "freshness window"})
	cmdutil.AddBoolVar(cmd.Flags(), &unbounded, "unbounded", false, "Walk every page (disables the default 100-page / 10 000-board cap)",
		clib.FlagExtra{Group: "Pagination", Terse: "walk every page"})
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
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "clear [resource]",
		Short: "Remove cached metadata",
		Long: "Remove local cache files for the active profile. Use it when cached metadata " +
			"is stale, malformed, or should be rebuilt on the next command.\n\n" +
			"With no argument, the command removes every cache file under the profile. " +
			"With a resource name such as `labels`, `projects`, or `boards`, it removes " +
			"only that cache file.\n\n" +
			"`--dry-run` reports what a live clear would remove without touching the files. " +
			"A live clear requires `--force` in headless, agent, or `--no-input` mode; an " +
			"interactive terminal proceeds without a prompt — the wipe is recoverable by " +
			"re-priming.",
		Args: cobra.MaximumNArgs(1),
		Example: `$ jira cache clear

# Target a single cached resource
$ jira cache clear boards

# Report what would be removed without removing anything
$ jira cache clear --dry-run --output=json

# Non-interactive (agent / script) clear
$ jira cache clear labels --force --output=json`,
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
			// Wiping local state is a mutation: headless, agent, and
			// --no-input callers must consent with --force. --dry-run never
			// mutates, so it stays open; an interactive terminal proceeds
			// without a prompt — a cleared cache is rebuilt by re-priming.
			det := cmdutil.DetectorFromContext(cmd)
			headless := !det.IsTTY || det.Agent || cmdutil.NoInputRequested(cmd)
			if !dryRun && !force && headless {
				return cli.NewCLIInputError(cli.InputForceRequired, "cache clear requires --force in headless / agent / --no-input mode")
			}
			key := cmdutil.CacheKeyForProfile(cmd, profile)
			if len(args) == 0 {
				var n int
				if dryRun {
					n, err = cache.CountProfile(key)
				} else {
					n, err = cache.ClearProfile(key)
				}
				if err != nil {
					return err
				}
				return cmdutil.WriteEnvelope(cmd, "cache.clear", map[string]any{
					"profile": profile.Name,
					"removed": n,
					"dry_run": dryRun,
				})
			}
			var ok bool
			if dryRun {
				ok, err = cache.Exists(key, args[0])
			} else {
				ok, err = cache.Clear(key, args[0])
			}
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "cache.clear", map[string]any{
				"profile":  profile.Name,
				"resource": args[0],
				"removed":  ok,
				"dry_run":  dryRun,
			})
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Report what would be removed without removing anything")
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm the clear in headless / agent / --no-input mode")
	return cmd
}

func isCacheClearResource(resource string) bool {
	return slices.Contains(cacheClearResources, resource)
}
