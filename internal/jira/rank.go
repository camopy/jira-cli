// Package jira — Rank service for PUT /rest/agile/1.0/issue/rank, the
// LexoRank reorder behind the web UI's backlog drag.
package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// RankChunkLimit is the endpoint's hard cap on issues per request. Callers
// ranking more keys chunk transparently, anchoring each later chunk after
// the last key of the one before it so the requested order survives.
const RankChunkLimit = 50

// RankService reorders backlog issues relative to an anchor issue.
type RankService interface {
	// Rank moves keys (at most RankChunkLimit, in the order given) so they
	// sit immediately before or after the anchor. Exactly one of before or
	// after must be non-empty.
	Rank(ctx context.Context, keys []string, before, after string) (*Response, error)
}

type rankService struct{ client *Client }

// NewRankService builds the rank service for a client.
func NewRankService(client *Client) RankService { return &rankService{client: client} }

type rankRequest struct {
	Issues          []string `json:"issues"`
	RankBeforeIssue string   `json:"rankBeforeIssue,omitempty"`
	RankAfterIssue  string   `json:"rankAfterIssue,omitempty"`
}

// RankEntry is one entry of the endpoint's 207 multi-status body: a group
// of issues that shared an outcome, with Jira's error strings when the
// group failed.
type RankEntry struct {
	Issues []string `json:"issues"`
	Errors []string `json:"errors"`
	Status int      `json:"status"`
}

type rankMultiStatus struct {
	Entries []RankEntry `json:"entries"`
}

// RankPartialError reports a 207 response whose entries include failures:
// some issues ranked, the listed ones did not. Validation-class — the rank
// call itself worked; specific issues were rejected.
type RankPartialError struct {
	Failed []RankEntry
}

func (e *RankPartialError) Error() string {
	parts := make([]string, 0, len(e.Failed))
	for _, entry := range e.Failed {
		part := strings.Join(entry.Issues, ", ")
		if len(entry.Errors) > 0 {
			part += " (" + strings.Join(entry.Errors, "; ") + ")"
		}
		parts = append(parts, part)
	}
	return "rank was rejected for " + strings.Join(parts, "; ")
}

// RankRejectedError types the endpoint's 400: the request shape was fine
// locally, so a 400 here means Jira refused the rank itself — commonly a
// project with no Jira Software board (no LexoRank field) or an anchor the
// caller cannot rank against. Scoped by status + endpoint, never by
// message text.
type RankRejectedError struct {
	Wrapped *APIError
}

func (e *RankRejectedError) Error() string { return e.Wrapped.Message }
func (e *RankRejectedError) Unwrap() error { return e.Wrapped }

func (s *rankService) Rank(ctx context.Context, keys []string, before, after string) (*Response, error) {
	if len(keys) == 0 {
		return nil, errors.New("rank: at least one issue key is required")
	}
	if len(keys) > RankChunkLimit {
		return nil, fmt.Errorf("rank: at most %d issues per request", RankChunkLimit)
	}
	if (before == "") == (after == "") {
		return nil, errors.New("rank: exactly one of a before or after anchor is required")
	}
	body := rankRequest{Issues: keys, RankBeforeIssue: before, RankAfterIssue: after}
	req, err := s.client.NewRequest(ctx, http.MethodPut, "rest/agile/1.0/issue/rank", body)
	if err != nil {
		return nil, err
	}
	var multi rankMultiStatus
	resp, err := s.client.Do(req, &multi)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.StatusCode == http.StatusBadRequest {
			return resp, &RankRejectedError{Wrapped: api}
		}
		return resp, err
	}
	if resp != nil && resp.Response != nil && resp.Response.StatusCode == http.StatusMultiStatus {
		failed := make([]RankEntry, 0, len(multi.Entries))
		for _, entry := range multi.Entries {
			if entry.Status >= http.StatusBadRequest {
				failed = append(failed, entry)
			}
		}
		if len(failed) > 0 {
			return resp, &RankPartialError{Failed: failed}
		}
	}
	return resp, nil
}
