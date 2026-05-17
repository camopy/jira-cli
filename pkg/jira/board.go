// Package jira — Board service for /rest/agile/1.0.
//
// Wire shapes and field semantics live in
// the boards HTTP contract and
// data-model.md. This file holds the Board struct, BoardScope helper,
// BoardService interface, and the drain/resolve implementations.
package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/matcra587/jira-cli/internal/cache"
)

// Board is a Jira agile board fetched via /rest/agile/1.0/board and (for
// the project list) /rest/agile/1.0/board/{id}/project. Pointer-typed
// nullable fields mirror Issue/Comment/Attachment: distinguishes "field
// absent" from "present and zero".
type Board struct {
	ID          *int     `json:"id,omitempty"`
	Self        *string  `json:"self,omitempty"`
	Name        *string  `json:"name,omitempty"`
	Type        *string  `json:"type,omitempty"`
	ProjectKeys []string `json:"project_keys"`
}

// BoardScope is what BoardService.ResolveOne returns: the resolved
// board plus the precedence path that selected it (one of "flag",
// "default_board", "none").
type BoardScope struct {
	Board      Board
	Precedence string
}

// JQLClause emits `project in (P1, P2, ...)` from the board's cached
// project keys. The bool result is the canonical "did this scope
// produce a clause?" signal — every consumer that needs to branch on
// emission (envelope's `applied` flag, JQL prepender) reads it from
// here so no caller has to re-derive `len(ProjectKeys) > 0`.
func (s BoardScope) JQLClause() (string, bool) {
	if len(s.Board.ProjectKeys) == 0 {
		return "", false
	}
	return "project in (" + strings.Join(s.Board.ProjectKeys, ", ") + ")", true
}

// BoardDrainOptions configures BoardService.ListAll. Defaults:
// PageSize 50, MaxPages 100, MaxResults 10 000. Set Unbounded to
// disable both bounds.
type BoardDrainOptions struct {
	PageSize   int
	MaxPages   int
	MaxResults int
	Unbounded  bool
}

// BoardDrainResult carries the outcome of ListAll: every board fetched,
// the rate-limit APIError that aborted the walk if one fired
// (partial-success), and the truncation reason if a bound stopped the
// walk short of isLast.
type BoardDrainResult struct {
	Boards          []*Board
	LastResp        *Response
	PagesFetched    int
	RateLimitHit    *APIError
	Truncated       bool
	TruncatedReason string
}

// ErrBoardNotFound is returned by ResolveOne when the cache contains no
// board matching the supplied name (case-insensitive exact match).
// CLI maps to exit 2 (not_found).
var ErrBoardNotFound = errors.New("board not found in cache")

// NormalizeBoardName strips leading/trailing whitespace from board
// names while preserving internal whitespace and Unicode verbatim.
// Called by the cache prime path before persisting each board, so the
// cache file holds clean names while the wire shape is left untouched.
func NormalizeBoardName(name string) string {
	return strings.TrimSpace(name)
}

// DefaultBoardMissingMessage returns the pinned wording the CLI emits
// when `default_board` references a name that does not resolve in the
// boards cache. Both arguments are interpolated verbatim — `profile`
// is the active profile name (e.g. "default" / "work"), `name` is the
// configured default_board value. Wording is pinned so the contract
// test can assert on it; any change here is a spec change too.
func DefaultBoardMissingMessage(profile, name string) string {
	return fmt.Sprintf(
		`default_board %q not found in boards cache — run "jira cache boards --refresh" or unset with "jira config set profiles.%s.default_board ''"`,
		name, profile,
	)
}

// AmbiguousBoardError is returned by ResolveOne when 2+ boards match
// the supplied name (e.g. same name in different projects). The cmd
// layer renders Candidates in the disambiguation envelope. CLI maps to
// exit 3 (validation).
type AmbiguousBoardError struct {
	Query      string
	Candidates []Board
}

func (e *AmbiguousBoardError) Error() string {
	return fmt.Sprintf("ambiguous board name %q — %d candidates", e.Query, len(e.Candidates))
}

// BoardService groups the agile board endpoints used by the boards
// cache primer, the `boards list` command, and the --board resolver.
type BoardService interface {
	List(ctx context.Context, opts *ListOptions) ([]Board, *Response, error)
	ListAll(ctx context.Context, opts BoardDrainOptions) (BoardDrainResult, error)
	ProjectsForBoard(ctx context.Context, boardID int) ([]string, *Response, error)
	ResolveOne(ctx context.Context, profile, name string) (BoardScope, error)
}

type boardService struct {
	client *Client
}

// NewBoardService binds a BoardService to the supplied client.
func NewBoardService(client *Client) BoardService {
	return &boardService{client: client}
}

// boardListWire is the paged response shape from /rest/agile/1.0/board.
type boardListWire struct {
	MaxResults int  `json:"maxResults"`
	StartAt    int  `json:"startAt"`
	IsLast     bool `json:"isLast"`
	Values     []struct {
		ID   *int    `json:"id,omitempty"`
		Self *string `json:"self,omitempty"`
		Name *string `json:"name,omitempty"`
		Type *string `json:"type,omitempty"`
	} `json:"values"`
}

// boardProjectWire is the paged response shape from /board/{id}/project.
type boardProjectWire struct {
	MaxResults int  `json:"maxResults"`
	StartAt    int  `json:"startAt"`
	IsLast     bool `json:"isLast"`
	Values     []struct {
		Key string `json:"key"`
	} `json:"values"`
}

// List fetches one page of /rest/agile/1.0/board. Project lists are
// NOT populated — the caller (cache primer) calls ProjectsForBoard per
// board to fill them in.
func (s *boardService) List(ctx context.Context, opts *ListOptions) ([]Board, *Response, error) {
	q := url.Values{}
	if opts != nil {
		if opts.MaxResults > 0 {
			q.Set("maxResults", strconv.Itoa(opts.MaxResults))
		}
		if opts.StartAt > 0 {
			q.Set("startAt", strconv.Itoa(opts.StartAt))
		}
	}
	path := "rest/agile/1.0/board"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var wire boardListWire
	resp, err := s.client.Do(req, &wire)
	if resp != nil {
		resp.MaxResults = wire.MaxResults
		resp.StartAt = wire.StartAt
		resp.IsLast = wire.IsLast
	}
	if err != nil {
		return nil, resp, err
	}
	out := make([]Board, len(wire.Values))
	for i, v := range wire.Values {
		out[i] = Board{
			ID:          v.ID,
			Self:        v.Self,
			Name:        v.Name,
			Type:        v.Type,
			ProjectKeys: []string{},
		}
	}
	return out, resp, nil
}

// ListAll drains the entire board set under the configured bounds.
// Mirrors CommentService.ListAll. Does NOT populate ProjectKeys — caller
// follows up with ProjectsForBoard if the cache wants them.
func (s *boardService) ListAll(ctx context.Context, opts BoardDrainOptions) (BoardDrainResult, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	var (
		out    BoardDrainResult
		offset int
	)
	for {
		page, resp, err := s.List(ctx, &ListOptions{StartAt: offset, MaxResults: pageSize})
		if err != nil {
			var apiErr *APIError
			if out.PagesFetched > 0 && errors.As(err, &apiErr) && apiErr.Type == ErrorTypeRateLimit {
				out.RateLimitHit = apiErr
				out.Truncated = true
				out.TruncatedReason = "rate_limit"
				return out, nil
			}
			return out, err
		}
		out.PagesFetched++
		out.LastResp = resp
		for i := range page {
			out.Boards = append(out.Boards, &page[i])
		}
		serverDone := resp == nil || resp.IsLast
		if !opts.Unbounded {
			if out.PagesFetched >= maxPages && !serverDone {
				out.Truncated = true
				out.TruncatedReason = "max_pages"
				return out, nil
			}
			if len(out.Boards) > maxResults || (len(out.Boards) == maxResults && !serverDone) {
				out.Truncated = true
				out.TruncatedReason = "max_results"
				if len(out.Boards) > maxResults {
					out.Boards = out.Boards[:maxResults]
				}
				return out, nil
			}
		}
		if serverDone {
			return out, nil
		}
		offset = nextOffset(offset, len(page), pageSize, resp.MaxResults)
	}
}

// ProjectsForBoard returns the project keys for a given board id,
// sorted ascending alphabetically (deterministic for cache+JQL).
func (s *boardService) ProjectsForBoard(ctx context.Context, boardID int) ([]string, *Response, error) {
	startAt := 0
	pageSize := 50
	var (
		keys     []string
		lastResp *Response
		pages    int
	)
	for {
		path := fmt.Sprintf("rest/agile/1.0/board/%d/project?startAt=%d&maxResults=%d", boardID, startAt, pageSize)
		req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, nil, err
		}
		var wire boardProjectWire
		resp, err := s.client.Do(req, &wire)
		if err != nil {
			return nil, resp, err
		}
		lastResp = resp
		pages++
		for _, v := range wire.Values {
			if v.Key != "" {
				keys = append(keys, v.Key)
			}
		}
		if wire.IsLast {
			break
		}
		if pages >= defaultMaxPages || len(keys) >= defaultMaxResults {
			return nil, lastResp, fmt.Errorf("board %d project pagination exceeded default bounds", boardID)
		}
		startAt = nextOffset(startAt, len(wire.Values), pageSize, wire.MaxResults)
	}
	// Sort ascending for deterministic envelope round-trips.
	slices.Sort(keys)
	return keys, lastResp, nil
}

// BoardsCacheFile is the on-disk envelope persisted under
// `${XDG_CACHE_HOME:-~/.cache}/jira-cli/<profile>/boards.json` per
// data-model.md > BoardsCache. Carried inside the generic cache.Entry's
// Data field so the existing internal/cache machinery (atomic write,
// 0600 perms, TTL freshness) round-trips it unchanged.
type BoardsCacheFile struct {
	FetchedAt         string  `json:"fetched_at"`
	TTLSeconds        int     `json:"ttl_seconds"`
	Truncated         bool    `json:"truncated"`
	TruncatedReason   string  `json:"truncated_reason"`
	PagesFetched      int     `json:"pages_fetched,omitempty"`
	RetryAfterSeconds int     `json:"retry_after_seconds,omitempty"`
	Items             []Board `json:"items"`
}

// DecodeBoardsCache parses the on-disk boards-cache payload. The earlier
// resolver tests stored the boards as a bare JSON array; the 003 prime
// path persists the BoardsCache envelope. This helper accepts both so
// the migration is invisible to callers.
func DecodeBoardsCache(data []byte) ([]Board, error) {
	if len(data) == 0 {
		return nil, nil
	}
	// Envelope shape ({items: [...]}) — peek for the items field.
	var env BoardsCacheFile
	if err := json.Unmarshal(data, &env); err == nil && env.Items != nil {
		return env.Items, nil
	}
	// Bare array shape (resolver-test fixtures).
	var arr []Board
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("board cache: payload is neither {items:[]} nor a bare array")
}

// ResolveOne does case-insensitive exact-only matching against the
// per-profile boards cache. 0 matches → ErrBoardNotFound (exit 2);
// 2+ matches → *AmbiguousBoardError (exit 3); 1 match → BoardScope.
//
// Substring fallback is intentionally NOT supported. Tab completion is
// the convenience layer.
func (s *boardService) ResolveOne(ctx context.Context, profile, name string) (BoardScope, error) {
	_ = ctx // resolution is local-only; ctx kept for future async hooks
	entry, ok, _, err := cache.Read(profile, "boards", 0)
	if err != nil {
		return BoardScope{}, fmt.Errorf("board resolve: %w", err)
	}
	if !ok {
		return BoardScope{}, fmt.Errorf("%w (boards cache empty — run `jira cache boards`)", ErrBoardNotFound)
	}
	boards, err := DecodeBoardsCache(entry.Data)
	if err != nil {
		return BoardScope{}, fmt.Errorf("board resolve: decode cache: %w", err)
	}

	want := strings.ToLower(strings.TrimSpace(name))
	matches := make([]Board, 0, 2)
	for _, b := range boards {
		if b.Name == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(*b.Name)) == want {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 0:
		return BoardScope{}, fmt.Errorf("%w: %q", ErrBoardNotFound, name)
	case 1:
		return BoardScope{Board: matches[0]}, nil
	default:
		return BoardScope{}, &AmbiguousBoardError{Query: name, Candidates: matches}
	}
}
