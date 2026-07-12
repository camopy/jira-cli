package jira

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ListOptions carries the paging inputs shared by the list endpoints. It spans
// both paging models Jira Cloud uses: StartAt/MaxResults for classic offset
// paging and NextPageToken for the token-cursor endpoints (enhanced search).
// A caller supplies whichever the target endpoint honors; the unused fields
// stay zero.
type ListOptions struct {
	StartAt       int
	MaxResults    int
	NextPageToken string
}

// QueryValues renders the offset-paging fields as URL query parameters,
// omitting each when non-positive so a zero-value ListOptions adds nothing to
// the query. NextPageToken is deliberately not emitted here — token endpoints
// place the cursor differently per endpoint.
func (o ListOptions) QueryValues() url.Values {
	q := url.Values{}
	if o.StartAt > 0 {
		q.Set("startAt", strconv.Itoa(o.StartAt))
	}
	if o.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(o.MaxResults))
	}
	return q
}

// QueryOptions is ListOptions plus a JQL string, for the read endpoints that
// take both paging and a query.
type QueryOptions struct {
	ListOptions
	JQL string
}

// Rate is the rate-limit state parsed from a response's headers. It is
// observability, not control flow: the retry logic keys off the HTTP status and
// Retry-After, while Reason and NearLimit only feed a caller's near-limit
// warning.
type Rate struct {
	Remaining         int
	RetryAfterSeconds int
	Reset             time.Time
	// Reason is Jira's RateLimit-Reason header (e.g. jira-burst-based,
	// jira-per-issue-on-write). Empty when absent. Carried for diagnostics;
	// the retry decision keys off the HTTP status and Retry-After, not this.
	Reason string
	// NearLimit reflects Jira's X-RateLimit-NearLimit header: the server's
	// own "you are approaching the limit" signal on an otherwise-successful
	// response. Surfaced as a non-fatal warning so a caller can ease off
	// before it turns into a 429.
	NearLimit bool
}

// Response wraps the raw *http.Response with the paging and rate metadata the
// services and envelope layer need. It is returned even alongside an error so a
// caller can still read RawBody and the rate state. The two paging models are
// distinguished by TokenPage: token-cursor endpoints set it and report no
// meaningful Total (see TotalKnown), classic offset endpoints leave it false.
type Response struct {
	Response   *http.Response
	StartAt    int
	MaxResults int
	Total      int
	// TotalKnown reports that Total was decoded from the endpoint's own
	// total field. Token-paged endpoints (enhanced search) never report
	// one, and a zero Total from them is fabrication, not fact — the
	// envelope only publishes a total when this is true.
	TotalKnown    bool
	IsLast        bool
	NextPageToken string
	TokenPage     bool
	Rate          Rate
	RawBody       json.RawMessage
}

// UpstreamRequestID returns Jira's own trace id for the exchange (the
// Atl-Traceid / X-ARequestId response header) so envelopes can carry a
// value Atlassian support can correlate. Empty when the response carried
// neither header.
func (r Response) UpstreamRequestID() string {
	return upstreamRequestID(r.Response)
}

// NextCursor returns the opaque cursor a caller passes to fetch the next page,
// or "" when the current page is the last. It papers over the two paging
// models: for a token endpoint it hands back the server's NextPageToken, for
// offset paging it computes the next StartAt and stops once IsLast or a known
// Total says the run is exhausted.
func (r Response) NextCursor() string {
	if r.TokenPage {
		return r.NextPageToken
	}
	if r.NextPageToken != "" {
		return r.NextPageToken
	}
	if r.IsLast || r.MaxResults <= 0 {
		return ""
	}
	next := r.StartAt + r.MaxResults
	if r.Total > 0 && next >= r.Total {
		return ""
	}
	return strconv.Itoa(next)
}
