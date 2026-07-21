package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/envelope"
	"github.com/matcra587/jira-cli/internal/jira"
)

// refreshOutcome is the per-resource success result. A failed resource is
// carried by the fan-out KeyResult error instead, so this only describes the
// fresh (skipped) and refreshed (refetched) outcomes.
type refreshOutcome struct {
	status     string // "fresh" or "refreshed"
	fromCache  bool
	count      int
	fetchedAt  time.Time
	durationMS int64
}

// cacheRefreshCommand refreshes every registered cache resource (or a named
// subset) in one pass. By default it is TTL-gated — a resource still fresh
// under its registry TTL is reported "fresh" and left untouched — and --force
// refetches everything. Resources are fetched with bounded concurrency; one
// resource failing does not abort the rest, and the partial-failure envelope
// keeps the successes while exiting non-zero.
func cacheRefreshCommand() *cobra.Command {
	var force, unbounded, dryRun bool
	var parallelism, ttlOverride int
	cmd := &cobra.Command{
		Use:   "refresh [resource...]",
		Short: "Refresh stale cache resources (or all with --force)",
		Long: "Refresh cached Jira metadata for the active profile. With no arguments, it " +
			"checks every registered resource; with resource names, it checks only those.\n\n" +
			"Fresh resources are skipped unless `--force` is set. Resources are refreshed " +
			"with bounded concurrency, and a failure for one resource is returned in the " +
			"structured envelope without discarding successful refreshes.",
		Example: `$ jira cache refresh

# Refresh everything, ignoring freshness
$ jira cache refresh --force

# Limit the refresh to named resources
$ jira cache refresh boards labels

# Refresh every resource and keep the result parseable
$ jira cache refresh --force --output=json`,
		Annotations: map[string]string{"clib": "dynamic-args='cacheresource'"},
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return registry.ResourceNames(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resources := args
			if len(resources) == 0 {
				resources = registry.ResourceNames()
			}
			for _, name := range resources {
				if _, found := registry.ByName(name); !found {
					return cli.NewCLIInputError(
						cli.InputArgValueInvalid,
						fmt.Sprintf("unknown cache resource %q; valid resources: %s", name, strings.Join(registry.ResourceNames(), ", ")),
					)
				}
			}

			var (
				client   *jira.Client
				hasURL   bool
				cacheKey string
			)
			if dryRun {
				// Local-only preview: resolve the cache key from config without
				// building a Jira client (which would require a credential), so
				// --dry-run never needs auth and never contacts Jira.
				cfg, cfgErr := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
				if cfgErr != nil {
					return cfgErr
				}
				cacheKey = cmdutil.CacheKeyForProfile(cmd, cmdutil.ActiveProfile(cmd, cfg))
			} else {
				c, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				client, hasURL = c, ok
				cacheKey = cmdutil.CacheKeyForProfile(cmd, profile)
			}

			fn := func(ctx context.Context, name string) (refreshOutcome, error) {
				return refreshResource(ctx, cacheKey, client, hasURL, name, force, unbounded, ttlOverride, dryRun)
			}
			results, err := cmdutil.FanOutKeysProgress(cmd.Context(), "cache.refresh", resources, parallelism, fn)
			if err != nil {
				return err
			}
			// The shared keyed-results envelope keys each row by resource
			// name, retains successes on partial failure, drives the plain
			// renderer, and maps the exit code to the highest per-resource
			// failure — identical to the multi-key issue commands.
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "cache.refresh", results,
				func(_ string, o refreshOutcome) any {
					return envelope.CacheRefreshRow{
						Status:     o.status,
						FromCache:  o.fromCache,
						Count:      o.count,
						FetchedAt:  o.fetchedAt.UTC().Format(time.RFC3339),
						DurationMS: o.durationMS,
						DryRun:     dryRun,
					}
				})
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &force, "force", false, "Refetch every resource regardless of freshness",
		clib.FlagExtra{Group: "Cache", Terse: "ignore freshness"})
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	cmdutil.AddIntVar(cmd.Flags(), &ttlOverride, "ttl-minutes", 0, "Freshness window override for all resources (0 = per-resource default)",
		clib.FlagExtra{Group: "Cache", Placeholder: "N", Terse: "freshness window"})
	cmdutil.AddBoolVar(cmd.Flags(), &unbounded, "unbounded", false, "For boards, walk every page (disables the default cap)",
		clib.FlagExtra{Group: "Pagination", Terse: "walk every page"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Report which resources are stale without refetching or writing (never contacts Jira)")
	return cmd
}

// refreshResource refreshes one resource: it serves a still-fresh cache
// untouched (unless force), otherwise refetches and writes. boards is fetched
// through PrimeAndCacheBoards because its cache is an object, not a flat list.
func refreshResource(ctx context.Context, cacheKey string, client *jira.Client, hasBaseURL bool, name string, force, unbounded bool, ttlOverride int, dryRun bool) (refreshOutcome, error) {
	start := time.Now()
	r, found := registry.ByName(name)
	if !found {
		return refreshOutcome{}, fmt.Errorf("unknown cache resource %q", name)
	}
	ttlMinutes := r.TTLMinutes
	if ttlOverride > 0 {
		ttlMinutes = ttlOverride
	}
	ttl := time.Duration(ttlMinutes) * time.Minute

	if !force {
		entry, present, stale, readErr := cache.Read(cacheKey, r.Name, ttl)
		if readErr == nil && present && !stale {
			return refreshOutcome{
				status:     "fresh",
				fromCache:  true,
				count:      cachedCount(r, entry.Data),
				fetchedAt:  entry.FetchedAt,
				durationMS: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	if dryRun {
		// Local-only preview: the resource is stale (or --force), so a live run
		// would refetch it. Report that without contacting Jira or writing; the
		// count and timestamp come from the existing cache entry when present.
		count, fetchedAt := 0, time.Time{}
		fromCache := false
		if entry, present, _, readErr := cache.Read(cacheKey, r.Name, ttl); readErr == nil && present {
			fromCache = true
			count, fetchedAt = cachedCount(r, entry.Data), entry.FetchedAt
		}
		return refreshOutcome{
			status:     "would-refresh",
			fromCache:  fromCache,
			count:      count,
			fetchedAt:  fetchedAt,
			durationMS: time.Since(start).Milliseconds(),
		}, nil
	}

	if !hasBaseURL {
		return refreshOutcome{}, fmt.Errorf("jira base URL is required to refresh %s", r.Name)
	}

	var (
		count     int
		fetchedAt time.Time
	)
	if r.Fetch == nil {
		// boards: bespoke prime (bounded pagination + per-board projects).
		file, entry, _, err := cmdutil.PrimeAndCacheBoards(ctx, cacheKey, client, ttlMinutes, unbounded)
		if err != nil {
			return refreshOutcome{}, err
		}
		count, fetchedAt = len(file.Items), entry.FetchedAt
	} else {
		data, err := r.Fetch(ctx, client)
		if err != nil {
			return refreshOutcome{}, err
		}
		entry, err := cache.Write(cacheKey, r.Name, data)
		if err != nil {
			return refreshOutcome{}, err
		}
		items, _ := decodeCacheList(data)
		count, fetchedAt = len(items), entry.FetchedAt
	}
	return refreshOutcome{
		status:     "refreshed",
		fromCache:  false,
		count:      count,
		fetchedAt:  fetchedAt,
		durationMS: time.Since(start).Milliseconds(),
	}, nil
}

// cachedCount returns the element count of a cached resource. boards stores an
// object (BoardsCacheFile) rather than a flat array, so it is counted by its
// items; every other resource is a JSON array.
func cachedCount(r registry.Resource, data json.RawMessage) int {
	if r.Fetch == nil {
		var file jira.BoardsCacheFile
		if json.Unmarshal(data, &file) == nil && len(file.Items) > 0 {
			return len(file.Items)
		}
		var arr []jira.Board // legacy bare-array fixtures
		if json.Unmarshal(data, &arr) == nil {
			return len(arr)
		}
		return 0
	}
	items, _ := decodeCacheList(data)
	return len(items)
}
