package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func keyOf(i *Issue) string {
	if i == nil || i.Key == nil {
		return ""
	}
	return *i.Key
}

// tokenPagedSearchClient serves a fixed number of token-paginated /search/jql
// pages: each page returns one issue and a nextPageToken until the last.
func tokenPagedSearchClient(t *testing.T, pages int) *Client {
	t.Helper()
	return newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %s, want /rest/api/3/search/jql", r.URL.Path)
		}
		var body struct {
			NextPageToken string `json:"nextPageToken"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		// page index is encoded in the token ("p<N>"); first request has none.
		idx := 0
		if body.NextPageToken != "" {
			idx, _ = strconv.Atoi(body.NextPageToken[1:])
		}
		key := "ISSUE-" + strconv.Itoa(idx)
		last := idx >= pages-1
		w.Header().Set("Content-Type", "application/json")
		resp := `{"issues":[{"key":"` + key + `"}],"isLast":` + strconv.FormatBool(last) + `}`
		if !last {
			resp = `{"issues":[{"key":"` + key + `"}],"isLast":false,"nextPageToken":"p` + strconv.Itoa(idx+1) + `"}`
		}
		_, _ = w.Write([]byte(resp))
	}))
}

// DrainSearch pulls every page until the server reports isLast, concatenating
// issues in order.
func TestDrainSearchWalksEveryPage(t *testing.T) {
	svc := NewSearchService(tokenPagedSearchClient(t, 3))
	issues, info, err := DrainSearch(context.Background(), svc, &SearchRequest{JQL: "project = ENG"}, DrainOptions{})
	if err != nil {
		t.Fatalf("DrainSearch: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("drained %d issues, want 3", len(issues))
	}
	if info.PagesFetched != 3 || info.Truncated {
		t.Fatalf("info = %+v, want 3 pages, not truncated", info)
	}
	if keyOf(issues[0]) != "ISSUE-0" || keyOf(issues[2]) != "ISSUE-2" {
		t.Fatalf("issue order wrong: %s … %s", keyOf(issues[0]), keyOf(issues[2]))
	}
}

// A max-pages bound stops the drain short of isLast and reports the truncation.
func TestDrainSearchStopsAtMaxPages(t *testing.T) {
	svc := NewSearchService(tokenPagedSearchClient(t, 10))
	issues, info, err := DrainSearch(context.Background(), svc, &SearchRequest{JQL: "project = ENG"}, DrainOptions{MaxPages: 2})
	if err != nil {
		t.Fatalf("DrainSearch: %v", err)
	}
	if !info.Truncated || info.TruncatedReason != "max_pages" {
		t.Fatalf("info = %+v, want truncated by max_pages", info)
	}
	if len(issues) != 2 {
		t.Fatalf("drained %d issues, want 2 (the bound)", len(issues))
	}
}

// A max-results bound stops the drain at the result cap (the trickier boundary:
// len(out) == maxResults while the server still has more) and reports it.
func TestDrainSearchStopsAtMaxResults(t *testing.T) {
	svc := NewSearchService(tokenPagedSearchClient(t, 10))
	issues, info, err := DrainSearch(context.Background(), svc, &SearchRequest{JQL: "project = ENG"}, DrainOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("DrainSearch: %v", err)
	}
	if !info.Truncated || info.TruncatedReason != "max_results" {
		t.Fatalf("info = %+v, want truncated by max_results", info)
	}
	if len(issues) != 2 {
		t.Fatalf("drained %d issues, want 2 (the result cap)", len(issues))
	}
}

// multiPerPageSearchClient serves `pages` token-paginated pages of `perPage`
// issues each, so a drain can cross the result cap mid-walk.
func multiPerPageSearchClient(t *testing.T, pages, perPage int) *Client {
	t.Helper()
	return newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NextPageToken string `json:"nextPageToken"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		idx := 0
		if body.NextPageToken != "" {
			idx, _ = strconv.Atoi(body.NextPageToken[1:])
		}
		items := make([]string, 0, perPage)
		for i := range perPage {
			items = append(items, `{"key":"P`+strconv.Itoa(idx)+`-`+strconv.Itoa(i)+`"}`)
		}
		last := idx >= pages-1
		body2 := `{"issues":[` + strings.Join(items, ",") + `],"isLast":` + strconv.FormatBool(last)
		if !last {
			body2 += `,"nextPageToken":"p` + strconv.Itoa(idx+1) + `"`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body2 + `}`))
	}))
}

// When a large page size makes BOTH bounds eligible on the same page, the
// result cap must win and clamp — never return more than maxResults issues
// mislabelled as a page-count stop. (Regression: max_pages was checked first.)
func TestDrainSearchResultCapWinsWhenBothBoundsEligible(t *testing.T) {
	// 2 issues/page; caps: 2 pages, 3 results. At page 2 the total (4) exceeds
	// the result cap AND the page count is reached — the cap must clamp to 3.
	svc := NewSearchService(multiPerPageSearchClient(t, 10, 2))
	issues, info, err := DrainSearch(context.Background(), svc, &SearchRequest{JQL: "project = ENG"}, DrainOptions{MaxPages: 2, MaxResults: 3})
	if err != nil {
		t.Fatalf("DrainSearch: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("drained %d issues, want exactly the 3-result cap (never exceed it)", len(issues))
	}
	if !info.Truncated || info.TruncatedReason != "max_results" {
		t.Fatalf("info = %+v, want truncated by max_results (the cap that bites)", info)
	}
}

// Unbounded ignores the bounds and walks to isLast.
func TestDrainSearchUnboundedIgnoresBounds(t *testing.T) {
	svc := NewSearchService(tokenPagedSearchClient(t, 5))
	issues, info, err := DrainSearch(context.Background(), svc, &SearchRequest{JQL: "project = ENG"}, DrainOptions{MaxPages: 2, Unbounded: true})
	if err != nil {
		t.Fatalf("DrainSearch: %v", err)
	}
	if info.Truncated || len(issues) != 5 {
		t.Fatalf("info = %+v, issues = %d; want all 5, not truncated", info, len(issues))
	}
}
