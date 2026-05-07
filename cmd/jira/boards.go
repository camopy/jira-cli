// `jira boards` command tree.
//
// Exposes a single read-only sub-command today: `jira boards list`.
// The cache primer lives in cache.go (`jira cache boards`); this file
// renders the cached list and primes transparently when the cache is
// empty so first-run UX requires no extra step.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// boardsCommand is the parent group for `jira boards <subcommand>`.
// Today only `list` exists; future feature slices may add `boards
// view`, `boards rename`, etc.
func boardsCommand() *cobra.Command {
	cmd := groupCommand("boards", "Browse the boards visible to this profile", "resources")
	cmd.AddCommand(boardsListCommand())
	return cmd
}

// boardsListCommand wires `jira boards list`.
//
// Read-first: serves from the per-profile boards cache when fresh,
// transparently primes via /rest/agile/1.0/board when the cache is
// empty, and re-primes whenever `--refresh` is supplied. The
// envelope `data` block matches contracts/envelope-shapes.md > boards
// list. Truncation surfaces both as `data.truncated` and as a
// `warnings[].type: "cache-truncated"` entry.
//
// `--raw` returns Atlassian's native paged shape verbatim.
func boardsListCommand() *cobra.Command {
	var refresh bool
	var ttlMinutes int
	var unbounded bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the boards visible to this profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			ttl := time.Duration(ttlMinutes) * time.Minute
			// --raw returns Atlassian's `/rest/agile/1.0/board` shape
			// verbatim. Bypasses the cache because the upstream payload
			// is what the user asked for, not the CLI's normalized
			// view.
			if raw, _ := cmd.Root().PersistentFlags().GetBool("raw"); raw {
				if !ok {
					return fmt.Errorf("jira base URL is required for boards.list")
				}
				return writeBoardsListRaw(cmd, client)
			}

			file, fetchedAt, fromCache, err := readOrPrimeBoardsCache(cmd, client, profile.Name, ok, ttl, ttlMinutes, refresh, unbounded)
			if err != nil {
				return err
			}

			// Stable ordering by id ascending. The primer already
			// sorts; cached payloads written by older callers may
			// not, so we re-sort defensively.
			sort.SliceStable(file.Items, func(i, j int) bool {
				return safeIntPtr(file.Items[i].ID) < safeIntPtr(file.Items[j].ID)
			})
			boards := boardsListEnvelope(file.Items)
			envelopeData := map[string]any{
				"boards": boards,
				"pagination": map[string]any{
					"total":           len(boards),
					"start_at":        0,
					"max_results":     len(boards),
					"is_last":         true,
					"next_page_token": nil,
				},
				"from_cache":       fromCache,
				"fetched_at":       fetchedAt.UTC().Format(time.RFC3339),
				"truncated":        file.Truncated,
				"truncated_reason": file.TruncatedReason,
			}
			warnings := boardsTruncationWarnings(file)
			return writeEnvelopeWithRawWarnings(cmd, "boards.list", envelopeData, warnings)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a re-prime even when the cache is fresh")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 60, "Freshness window before automatic refresh")
	cmd.Flags().BoolVar(&unbounded, "unbounded", false, "Walk every page (disables the default 100-page / 10 000-board cap)")
	// Read-only commands accept --dry-run as a no-op so callers that
	// pipeline mutation + read commands can pass the same flag set to
	// both without conditioning on command type.
	var dryRun bool
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "No-op on read-only commands; accepted for flag-set consistency")
	return cmd
}

// readOrPrimeBoardsCache is the read-first cache resolver that powers
// `boards list`. Returns the parsed BoardsCacheFile, its on-disk
// FetchedAt timestamp, and a `fromCache` marker indicating whether
// this invocation served from disk (true) or primed from API (false).
//
// Empty / stale cache + no --refresh → still primes, because the
// "transparent first-run" UX would otherwise leave new users with an
// empty list. --refresh always primes regardless of staleness.
func readOrPrimeBoardsCache(cmd *cobra.Command, client *jira.Client, profileName string, hasClient bool, ttl time.Duration, ttlMinutes int, refresh, unbounded bool) (jira.BoardsCacheFile, time.Time, bool, error) {
	if !refresh {
		entry, present, stale, readErr := cache.Read(profileName, "boards", ttl)
		if readErr == nil && present && !stale {
			items, err := jira.DecodeBoardsCache(entry.Data)
			if err != nil {
				return jira.BoardsCacheFile{}, time.Time{}, false, fmt.Errorf("boards.list: decode cache: %w", err)
			}
			file := boardsCacheFileFromEntry(entry.Data, items)
			return file, entry.FetchedAt, true, nil
		}
	}
	if !hasClient {
		return jira.BoardsCacheFile{}, time.Time{}, false, fmt.Errorf("jira base URL is required for boards.list")
	}
	file, _, err := primeBoards(cmd.Context(), client, ttlMinutes, unbounded)
	if err != nil {
		return jira.BoardsCacheFile{}, time.Time{}, false, err
	}
	body, err := json.Marshal(file)
	if err != nil {
		return jira.BoardsCacheFile{}, time.Time{}, false, fmt.Errorf("boards.list: marshal cache: %w", err)
	}
	entry, err := cache.Write(profileName, "boards", body)
	if err != nil {
		return jira.BoardsCacheFile{}, time.Time{}, false, fmt.Errorf("boards.list: write cache: %w", err)
	}
	return file, entry.FetchedAt, false, nil
}

// boardsCacheFileFromEntry reconstructs the BoardsCacheFile envelope
// from a cache.Entry.Data payload. Tolerates both the wrapped
// {items,...} form (current primer) and the bare-array form (legacy
// resolver-test fixtures) via DecodeBoardsCache.
func boardsCacheFileFromEntry(raw json.RawMessage, items []jira.Board) jira.BoardsCacheFile {
	var file jira.BoardsCacheFile
	// Best-effort: parse the wrapped envelope to recover truncation
	// markers. If the on-disk shape is a bare array, the unmarshal
	// noop's and Truncated/TruncatedReason stay false/"".
	_ = json.Unmarshal(raw, &file)
	if items == nil {
		items = []jira.Board{}
	}
	file.Items = items
	return file
}

// boardsTruncationWarnings maps a cached truncation marker onto the
// envelope's warnings[] entry per contracts/envelope-shapes.md.
// Returns nil when nothing was truncated.
func boardsTruncationWarnings(file jira.BoardsCacheFile) []map[string]any {
	if !file.Truncated {
		return nil
	}
	switch file.TruncatedReason {
	case "max_pages":
		return []map[string]any{{
			"type":        "cache-truncated",
			"resource":    "boards",
			"reason":      "max_pages",
			"limit":       100,
			"message":     "boards cache truncated by max_pages; re-run with --unbounded to fetch every board",
			"remediation": "Re-run with --unbounded if you need every board.",
		}}
	case "max_results":
		return []map[string]any{{
			"type":        "cache-truncated",
			"resource":    "boards",
			"reason":      "max_results",
			"limit":       10_000,
			"message":     "boards cache truncated by max_results; re-run with --unbounded to fetch every board",
			"remediation": "Re-run with --unbounded if you need every board.",
		}}
	case "rate_limit":
		return []map[string]any{{
			"type":        "rate-limit-during-paginate",
			"resource":    "boards",
			"message":     "boards cache truncated by rate_limit; re-run after the rate-limit window resets",
			"remediation": "Re-run `jira cache boards --refresh` after the rate-limit window resets.",
		}}
	}
	return nil
}

// boardsListEnvelope flattens jira.Board into the wire shape
// documented in contracts/envelope-shapes.md > boards list. The
// pointer-typed fields collapse to their zero values when absent so
// agents don't need null-handling for the common board case.
func boardsListEnvelope(items []jira.Board) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, b := range items {
		row := map[string]any{}
		if b.ID != nil {
			row["id"] = *b.ID
		} else {
			row["id"] = 0
		}
		if b.Name != nil {
			row["name"] = *b.Name
		} else {
			row["name"] = ""
		}
		if b.Type != nil {
			row["type"] = *b.Type
		} else {
			row["type"] = ""
		}
		if b.ProjectKeys == nil {
			row["project_keys"] = []string{}
		} else {
			row["project_keys"] = b.ProjectKeys
		}
		out = append(out, row)
	}
	return out
}

// safeIntPtr dereferences a *int with a 0 fallback for nil so
// SliceStable comparisons don't panic on a server response missing
// the id field.
func safeIntPtr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// writeBoardsListRaw emits Atlassian's native /rest/agile/1.0/board
// page verbatim. Skips the cache because --raw is upstream-shape
// transparency.
func writeBoardsListRaw(cmd *cobra.Command, client *jira.Client) error {
	req, err := client.NewRequest(cmd.Context(), http.MethodGet, "rest/agile/1.0/board", nil)
	if err != nil {
		return err
	}
	var raw json.RawMessage
	if _, err := client.Do(req, &raw); err != nil {
		return err
	}
	return cli.WriteRaw(cmd.OutOrStdout(), raw)
}

// writeEnvelopeWithRawWarnings is the shared helper in commands.go for
// emitting standard envelopes alongside free-form warning shapes (e.g.
// the cache-truncated warning) that don't fit cli.Warning's typed fields.
