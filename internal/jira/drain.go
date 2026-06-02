package jira

import (
	"context"
)

// DrainOptions configures DrainSearch — the bounded `--all` consumer.
//
// Defaults: max-pages 100, max-results-total 10,000. Override either
// with explicit positive values; set Unbounded to disable both (CLI
// requires --unbounded; the TUI never sets it).
type DrainOptions struct {
	MaxPages   int
	MaxResults int
	Unbounded  bool
}

// DrainInfo describes how DrainSearch terminated. Truncated reports
// whether a bound stopped consumption short of isLast.
type DrainInfo struct {
	PagesFetched    int
	Truncated       bool
	TruncatedReason string
}

const (
	defaultMaxPages   = 100
	defaultMaxResults = 10_000
)

// DrainSearch pulls every page from a SearchService.JQL call, respecting
// the configured bounds. Unbounded=true bypasses the bounds entirely;
// this is what the CLI's --unbounded flag wires.
func DrainSearch(ctx context.Context, svc SearchService, req *SearchRequest, opts DrainOptions) ([]*Issue, DrainInfo, error) {
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	var (
		out   []*Issue
		info  DrainInfo
		token string
	)
	for {
		page := *req
		if token != "" {
			page.NextPageToken = token
		}
		issues, resp, err := svc.JQL(ctx, &page)
		if err != nil {
			return nil, info, err
		}
		info.PagesFetched++
		out = append(out, issues...)
		serverDone := resp == nil || resp.IsLast || resp.NextPageToken == ""

		// Bound checks (skipped under Unbounded). max_results is checked first
		// so the result cap always holds: with a large page size both bounds can
		// be eligible on the same page, and checking max_pages first would
		// return more than maxResults issues mislabelled as a page-count stop.
		if !opts.Unbounded {
			if len(out) > maxResults || (len(out) == maxResults && !serverDone) {
				info.Truncated = true
				info.TruncatedReason = "max_results"
				if len(out) > maxResults {
					out = out[:maxResults]
				}
				return out, info, nil
			}
			if info.PagesFetched >= maxPages && !serverDone {
				info.Truncated = true
				info.TruncatedReason = "max_pages"
				return out, info, nil
			}
		}

		// Server says we're done.
		if serverDone {
			return out, info, nil
		}
		token = resp.NextPageToken
	}
}
