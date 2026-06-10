package jira

import "context"

// PageCursor is an opaque continuation handle for page-at-a-time consumers
// (the TUI's scroll-to-fetch). Pagination mechanics live in this package per
// the pagination guardrail: callers only test More() and hand the cursor back
// to ListIssuesPage — they never see the raw token.
type PageCursor struct {
	token string
	more  bool
}

// More reports whether the server has another page beyond this cursor.
func (c PageCursor) More() bool { return c.more }

// IssuePageLister is the one-method surface ListIssuesPage needs.
type IssuePageLister interface {
	List(ctx context.Context, opts *IssueListOptions) ([]*Issue, *Response, error)
}

// ListIssuesPage runs one page of an issue search and returns the cursor for
// the next call. Pass the zero PageCursor for the first page; a returned
// cursor with More() == false means the result set is complete.
func ListIssuesPage(ctx context.Context, svc IssuePageLister, opts *IssueListOptions, cur PageCursor) ([]*Issue, PageCursor, error) {
	o := *opts
	o.NextPageToken = cur.token
	issues, resp, err := svc.List(ctx, &o)
	if err != nil {
		return nil, PageCursor{}, err
	}
	var next PageCursor
	if resp != nil && !resp.IsLast && resp.NextPageToken != "" {
		next = PageCursor{token: resp.NextPageToken, more: true}
	}
	return issues, next, nil
}

// CursorForTest fabricates a continuation cursor so consumers outside this
// package can exercise fetch-more paths in tests without touching raw tokens.
// An empty token yields the done cursor.
func CursorForTest(token string) PageCursor {
	return PageCursor{token: token, more: token != ""}
}
