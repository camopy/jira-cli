package jira

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type ListOptions struct {
	StartAt       int
	MaxResults    int
	NextPageToken string
}

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

type QueryOptions struct {
	ListOptions
	JQL string
}

type Rate struct {
	Remaining         int
	RetryAfterSeconds int
	Reset             time.Time
	// Reason is Jira's RateLimit-Reason header (e.g. jira-burst-based,
	// jira-per-issue-on-write). Empty when absent. Carried for diagnostics;
	// the retry decision keys off the HTTP status and Retry-After, not this.
	Reason string
}

type Response struct {
	Response      *http.Response
	StartAt       int
	MaxResults    int
	Total         int
	IsLast        bool
	NextPageToken string
	TokenPage     bool
	Rate          Rate
	RawBody       json.RawMessage
}

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
