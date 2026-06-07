package cmdutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Cache source-state labels recorded on an envelope's data.cache_source_state
// field, describing how a cached resource was served (or why it was refetched).
const (
	CacheStateEmpty     = "empty"
	CacheStateFresh     = "fresh"
	CacheStateMalformed = "malformed"
	CacheStateMissing   = "missing"
	CacheStateRefresh   = "refresh"
	CacheStateStale     = "stale"
)

// CacheReadOrFetch is the read-then-fetch helper every cache subcommand uses.
// Returns the JSON-encoded resource bytes, whether the value was served from
// disk, and the state observed before any fetch.
func CacheReadOrFetch(profile, resource string, ttl time.Duration, refresh bool, fetch func() (json.RawMessage, error)) (json.RawMessage, bool, time.Time, string, error) {
	sourceState := CacheStateRefresh
	if !refresh {
		entry, ok, stale, err := cache.Read(profile, resource, ttl)
		switch {
		case err != nil:
			sourceState = CacheStateMalformed
		case ok && !stale:
			return entry.Data, true, entry.FetchedAt, CacheStateFresh, nil
		case ok && stale:
			sourceState = CacheStateStale
		default:
			sourceState = CacheStateMissing
		}
	}
	data, err := fetch()
	if err != nil {
		return nil, false, time.Time{}, sourceState, err
	}
	entry, err := cache.Write(profile, resource, data)
	if err != nil {
		return nil, false, time.Time{}, sourceState, err
	}
	return entry.Data, false, entry.FetchedAt, sourceState, nil
}

// CacheStateForCount collapses the source state to "empty" when the resource
// carried no records, otherwise returns the observed state.
func CacheStateForCount(sourceState string, count int) string {
	if count == 0 {
		return CacheStateEmpty
	}
	return sourceState
}

// AddCacheStateFields writes the cache_state / cache_source_state / cache_empty
// trio onto an envelope's data map.
func AddCacheStateFields(data map[string]any, sourceState string, count int) {
	data["cache_state"] = CacheStateForCount(sourceState, count)
	data["cache_source_state"] = sourceState
	data["cache_empty"] = count == 0
}

// PrimeAndCacheBoards primes the boards for the profile and writes them to the
// per-profile boards cache, returning the normalized file, the written cache
// entry, and any prime warnings. It is the shared fetch-and-store step behind
// `cache boards --refresh` and the post-login boards-cache warm.
func PrimeAndCacheBoards(ctx context.Context, cacheKey string, client *jira.Client, ttlMinutes int, unbounded bool) (jira.BoardsCacheFile, cache.Entry, []map[string]any, error) {
	file, warnings, err := PrimeBoards(ctx, client, ttlMinutes, unbounded)
	if err != nil {
		return jira.BoardsCacheFile{}, cache.Entry{}, warnings, err
	}
	body, err := json.Marshal(file)
	if err != nil {
		return file, cache.Entry{}, warnings, fmt.Errorf("cache.boards: marshal cache: %w", err)
	}
	entry, err := cache.Write(cacheKey, "boards", body)
	if err != nil {
		return file, cache.Entry{}, warnings, fmt.Errorf("cache.boards: write: %w", err)
	}
	return file, entry, warnings, nil
}

// PrimeBoards fetches every board (and its project keys) for the profile and
// returns the normalized cache file plus any warnings describing data dropped
// during the prime (bad records, unsafe project keys, partial pagination).
func PrimeBoards(ctx context.Context, client *jira.Client, ttlMinutes int, unbounded bool) (jira.BoardsCacheFile, []map[string]any, error) {
	svc := ServicesForClient(client).Board()
	res, err := svc.ListAll(ctx, jira.BoardDrainOptions{Unbounded: unbounded})
	if err != nil {
		return jira.BoardsCacheFile{}, nil, err
	}

	// One pass: dereference, normalize name, enforce data-model
	// invariants (ID > 0, trimmed Name non-empty), fetch project keys,
	// strip any key carrying JQL meta-characters. Bad-record drops,
	// project-fetch failures, and key drops are counted separately so
	// the warning surface tells the user exactly what was lost.
	items := make([]jira.Board, 0, len(res.Boards))
	var (
		droppedRecords  int
		droppedKeys     int
		failedFetches   []int
		projectKeysByID = map[int][]string{}
	)
	for _, b := range res.Boards {
		if b == nil {
			continue
		}
		clean := *b
		if clean.Name != nil {
			n := jira.NormalizeBoardName(*clean.Name)
			clean.Name = &n
		}
		if clean.ID == nil || *clean.ID <= 0 ||
			clean.Name == nil || xstrings.IsBlank(*clean.Name) {
			droppedRecords++
			continue
		}
		id := *clean.ID
		keys, found := projectKeysByID[id]
		if !found {
			var perr error
			keys, _, perr = svc.ProjectsForBoard(ctx, id)
			if perr != nil {
				failedFetches = append(failedFetches, id)
				clean.ProjectKeys = []string{}
				items = append(items, clean)
				continue
			}
			projectKeysByID[id] = append([]string(nil), keys...)
		}
		clean.ProjectKeys, droppedKeys = filterJQLSafeKeys(keys, droppedKeys)
		items = append(items, clean)
	}

	ttl := ttlMinutes * 60
	file := jira.BoardsCacheFile{
		FetchedAt:         time.Now().UTC().Format(time.RFC3339),
		TTLSeconds:        ttl,
		Truncated:         res.Truncated,
		TruncatedReason:   res.TruncatedReason,
		PagesFetched:      res.PagesFetched,
		RetryAfterSeconds: retryAfterSeconds(res.RateLimitHit),
		Items:             items,
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
			"type":                "rate-limit-during-paginate",
			"resource":            "boards",
			"pages_fetched":       res.PagesFetched,
			"retry_after_seconds": retryAfterSeconds(res.RateLimitHit),
			"remediation":         "Re-run `jira cache boards --refresh` after the rate-limit window resets.",
		})
	}
	if droppedRecords > 0 {
		warnings = append(warnings, map[string]any{
			"type":     "bad-records-dropped",
			"resource": "boards",
			"count":    droppedRecords,
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
	if len(failedFetches) > 0 {
		warnings = append(warnings, map[string]any{
			"type":        "project-fetch-failed",
			"resource":    "boards",
			"board_ids":   failedFetches,
			"count":       len(failedFetches),
			"remediation": "Re-run `jira cache boards --refresh` to retry the failed boards.",
		})
	}
	return file, warnings, nil
}

// filterJQLSafeKeys drops project keys that would corrupt the JQL emitted by
// BoardScope.JQLClause (`project in (P1, P2, ...)`). The clause does not quote
// keys, so any key carrying whitespace, commas, parens, quotes, or newlines
// must be filtered before it reaches the wire. Atlassian constrains keys
// server-side; this is defense in depth against malformed wire data.
func filterJQLSafeKeys(keys []string, droppedSoFar int) ([]string, int) {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, " \t\n\r,()'\"") {
			droppedSoFar++
			continue
		}
		out = append(out, k)
	}
	return out, droppedSoFar
}

func retryAfterSeconds(apiErr *jira.APIError) int {
	if apiErr == nil {
		return 0
	}
	return apiErr.RetryAfterSeconds
}
