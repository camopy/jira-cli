package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
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
	var force, unbounded bool
	var parallelism, ttlOverride int
	cmd := &cobra.Command{
		Use:   "refresh [resource...]",
		Short: "Refresh stale cache resources (or all with --force)",
		Long: "Refreshes cached Jira metadata. With no argument, refreshes every resource; " +
			"with resource names, only those. Fresh resources are skipped unless --force is given.",
		Example: `# Refresh every stale resource
$ jira cache refresh

# Refresh everything, ignoring freshness
$ jira cache refresh --force

# Refresh just boards and labels
$ jira cache refresh boards labels`,
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

			client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			cacheKey := cmdutil.CacheKeyForProfile(cmd, profile)

			fn := func(ctx context.Context, name string) (refreshOutcome, error) {
				return refreshResource(ctx, cacheKey, client, ok, name, force, unbounded, ttlOverride)
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
					return map[string]any{
						"status":      o.status,
						"from_cache":  o.fromCache,
						"count":       o.count,
						"fetched_at":  o.fetchedAt.UTC().Format(time.RFC3339),
						"duration_ms": o.durationMS,
					}
				})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Refetch every resource regardless of freshness")
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	cmd.Flags().IntVar(&ttlOverride, "ttl-minutes", 0, "Freshness window override for all resources (0 = per-resource default)")
	cmd.Flags().BoolVar(&unbounded, "unbounded", false, "For boards, walk every page (disables the default cap)")
	return cmd
}

// refreshResource refreshes one resource: it serves a still-fresh cache
// untouched (unless force), otherwise refetches and writes. boards is fetched
// through PrimeAndCacheBoards because its cache is an object, not a flat list.
func refreshResource(ctx context.Context, cacheKey string, client *jira.Client, hasBaseURL bool, name string, force, unbounded bool, ttlOverride int) (refreshOutcome, error) {
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
