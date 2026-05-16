package jira

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.atlassian.com/"
	maxErrorBodyBytes = 4096
)

type ErrorType string

const (
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeServer     ErrorType = "server"
)

// APIError is the typed error returned for any non-2xx Jira REST
// response or transport failure. Beyond the HTTP status and a display
// message it preserves the schema-backed fields of Jira Cloud's
// ErrorCollection body (errorMessages[], errors map, status) and the
// rate-limit Retry-After header, all as optional metadata. Jira exposes
// no stable machine error code, so APIError carries none — callers
// derive a normalized code from the HTTP status.
type APIError struct {
	StatusCode int
	Type       ErrorType
	Message    string
	// ErrorMessages mirrors the Jira ErrorCollection.errorMessages array
	// (general, non-field-scoped human messages). Nil when the body
	// carried none. Never used as a branch target — wording is not
	// contractual.
	ErrorMessages []string
	// FieldErrors mirrors the Jira ErrorCollection.errors map
	// (field-name -> single human message). Nil when the body carried
	// none.
	FieldErrors map[string]string
	// UpstreamStatus is the integer ErrorCollection.status field when the
	// body carried one. Zero when absent.
	UpstreamStatus int
	// RetryAfterSeconds is the Retry-After header value on a 429
	// response. Zero when the header was absent — callers fall back to
	// exponential backoff with jitter.
	RetryAfterSeconds int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira: %s (%d): %s", e.Type, e.StatusCode, e.Message)
}

// errorCollection is the Jira Cloud REST v3 standard error body shape
// (#/components/schemas/ErrorCollection). All fields are optional.
type errorCollection struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
	Status        int               `json:"status"`
}

// parseErrorCollection best-effort decodes a Jira error body into the
// ErrorCollection shape. A body that does not parse as ErrorCollection
// (HTML maintenance page, empty body) yields a zero value — the caller
// keeps the raw message instead.
func parseErrorCollection(body []byte) errorCollection {
	var ec errorCollection
	if len(body) == 0 || !json.Valid(body) {
		return ec
	}
	_ = json.Unmarshal(body, &ec)
	return ec
}

type Client struct {
	client      *http.Client
	baseURL     *url.URL
	bearerToken string
	basicEmail  string
	basicToken  string
	debug       bool
	readOnly    bool
}

type Option func(*Client)

func NewClient(opts ...Option) *Client {
	u, _ := url.Parse(defaultBaseURL)
	c := &Client{client: &http.Client{Timeout: 30 * time.Second}, baseURL: u}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func MTLSHTTPClient(certFile, keyFile string, timeout time.Duration) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.client = h
		}
	}
}

func WithHTTPTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout <= 0 {
			return
		}
		if c.client == nil || c.client == http.DefaultClient {
			c.client = &http.Client{Timeout: timeout}
			return
		}
		copy := *c.client
		copy.Timeout = timeout
		c.client = &copy
	}
}

func WithBaseURL(raw string) Option {
	return func(c *Client) {
		if u, err := url.Parse(raw); err == nil {
			c.baseURL = u
		}
	}
}

func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.bearerToken = token
	}
}

func WithBasicAuth(email, token string) Option {
	return func(c *Client) {
		c.basicEmail = email
		c.basicToken = token
	}
}

func WithDebug(debug bool) Option {
	return func(c *Client) {
		c.debug = debug
	}
}

// WithReadOnly causes Do to refuse any non-safe HTTP method (POST / PUT /
// PATCH / DELETE) with a validation-typed APIError. Single point of control
// — the CLI sets this once based on env / per-profile config and every
// mutation in every command path is automatically gated.
func WithReadOnly(readOnly bool) Option {
	return func(c *Client) {
		c.readOnly = readOnly
	}
}

func (c *Client) NewRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	rel, err := url.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, err
	}
	u := c.baseURL.ResolveReference(rel)
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.SignRequest(req)
	return req, nil
}

// SignRequest applies the active profile's auth headers to req.
// Services that build requests outside NewRequest (multipart upload,
// raw-body posts, streaming download) MUST call this rather than
// re-implementing the bearer/basic-auth selection inline so the auth
// surface stays consistent if new modes (OAuth / PAT / session) land.
func (c *Client) SignRequest(req *http.Request) {
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	if c.basicEmail != "" || c.basicToken != "" {
		req.SetBasicAuth(c.basicEmail, c.basicToken)
	}
}

// BaseURL exposes the resolved base URL so services that need to build
// fully-qualified URLs without going through NewRequest (multipart,
// streaming download) can ResolveReference against the same root the
// rest of the package uses.
func (c *Client) BaseURL() *url.URL {
	return c.baseURL
}

func (c *Client) Do(req *http.Request, out any) (*Response, error) {
	if c.readOnly && isMutationRequest(req) {
		return nil, &APIError{
			Type:    ErrorTypeValidation,
			Message: "read-only mode is active (JIRA_READ_ONLY env or profile read_only=true); refusing " + req.Method + " " + req.URL.Path,
		}
	}
	if c.debug {
		c.dumpRequest(req)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if c.debug {
		c.dumpResponseHead(res)
	}
	resp := &Response{Response: res, Rate: parseRate(res)}
	body, readErr := io.ReadAll(res.Body)
	if c.debug {
		c.dumpResponseBody(body)
	}
	if readErr != nil {
		// A network blip mid-read MUST surface as a typed server error
		// rather than degrade into a JSON-decode failure downstream.
		// Caller maps server → exit 5.
		return resp, &APIError{
			StatusCode: res.StatusCode,
			Type:       ErrorTypeServer,
			Message:    "response body read failed: " + readErr.Error(),
		}
	}
	if len(body) > 0 {
		resp.RawBody = append(resp.RawBody[:0], body...)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msgBody := body
		if len(msgBody) > maxErrorBodyBytes {
			msgBody = msgBody[:maxErrorBodyBytes]
		}
		ec := parseErrorCollection(msgBody)
		apiErr := &APIError{
			StatusCode:        res.StatusCode,
			Type:              classifyStatus(res.StatusCode),
			Message:           strings.TrimSpace(string(msgBody)),
			ErrorMessages:     ec.ErrorMessages,
			FieldErrors:       ec.Errors,
			UpstreamStatus:    ec.Status,
			RetryAfterSeconds: resp.Rate.RetryAfterSeconds,
		}
		return resp, apiErr
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil && len(body) > 0 {
			// A 2xx response with an unparseable body (e.g. an HTML
			// maintenance page served with status 200) is a server-side
			// failure, not a local validation error.  Wrap the decode error so
			// that outputErrorFor's type-assert on *APIError can route it to
			// exit 5 instead of the default exit 3.
			snippet := body
			if len(snippet) > maxErrorBodyBytes {
				snippet = snippet[:maxErrorBodyBytes]
			}
			return resp, &APIError{
				StatusCode: res.StatusCode,
				Type:       ErrorTypeServer,
				Message:    "response body is not valid JSON: " + err.Error() + "; body prefix: " + strings.TrimSpace(string(snippet)),
			}
		}
	}
	return resp, nil
}

// dumpRequest writes the outbound request to stderr in clog DBG style.
// The Authorization header is redacted to "REDACTED" so token material
// never reaches the log. Body is reset after read so the http.Client
// still sees it on the actual send.
func (c *Client) dumpRequest(req *http.Request) {
	headers := redactHeaders(req.Header)
	bodyText := ""
	if req.Body != nil && req.GetBody != nil {
		// req.Body has already been used to build the request; rewind
		// via GetBody for the dump. http.NewRequestWithContext sets
		// GetBody when the body is a *bytes.Reader.
		body, err := req.GetBody()
		if err == nil && body != nil {
			data, _ := io.ReadAll(body)
			_ = body.Close()
			bodyText = string(data)
		}
	}
	fmt.Fprintf(os.Stderr, "DBG ▶ %s %s\n", req.Method, req.URL.String())
	for k, v := range headers {
		fmt.Fprintf(os.Stderr, "DBG    %s: %s\n", k, strings.Join(v, ", "))
	}
	if bodyText != "" {
		fmt.Fprintf(os.Stderr, "DBG    body: %s\n", oneLineSnippet(bodyText, 1024))
	}
}

// dumpResponseHead writes status + headers to stderr.
func (c *Client) dumpResponseHead(res *http.Response) {
	fmt.Fprintf(os.Stderr, "DBG ◀ %d %s\n", res.StatusCode, res.Status)
	for k, v := range redactHeaders(res.Header) {
		fmt.Fprintf(os.Stderr, "DBG    %s: %s\n", k, strings.Join(v, ", "))
	}
}

// dumpResponseBody writes the response body — truncated to keep logs
// usable. Pretty-prints JSON when the body parses cleanly.
func (c *Client) dumpResponseBody(body []byte) {
	if len(body) == 0 {
		fmt.Fprintln(os.Stderr, "DBG    body: (empty)")
		return
	}
	fmt.Fprintf(os.Stderr, "DBG    body: %s\n", oneLineSnippet(string(body), 2048))
}

// redactHeaders returns a copy of h with sensitive headers replaced by
// "REDACTED" so debug logs never contain bearer tokens or basic-auth
// material.
func redactHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		switch strings.ToLower(k) {
		case "authorization", "cookie", "x-atlassian-token":
			out[k] = []string{"REDACTED"}
		default:
			out[k] = v
		}
	}
	return out
}

// oneLineSnippet flattens whitespace and truncates so debug output
// stays readable in a terminal.
func oneLineSnippet(s string, maxLen int) string {
	flat := strings.Join(strings.Fields(s), " ")
	if len(flat) > maxLen {
		flat = flat[:maxLen] + "…(truncated)"
	}
	return flat
}

func parseRate(res *http.Response) Rate {
	remaining, _ := strconv.Atoi(res.Header.Get("X-RateLimit-Remaining"))
	retryAfter, _ := strconv.Atoi(res.Header.Get("Retry-After"))
	return Rate{Remaining: remaining, RetryAfterSeconds: retryAfter}
}

// isMutationRequest decides whether a request actually changes server
// state. Method alone isn't enough — Jira's /search/jql endpoint uses
// POST so callers can send large JQL bodies, but it's a pure read.
func isMutationRequest(req *http.Request) bool {
	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	// Whitelist Jira read endpoints that happen to use POST.
	if strings.Contains(req.URL.Path, "/search/") {
		return false
	}
	return true
}

func classifyStatus(code int) ErrorType {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrorTypeAuth
	case code == http.StatusNotFound:
		return ErrorTypeNotFound
	case code == http.StatusTooManyRequests:
		return ErrorTypeRateLimit
	case code == http.StatusRequestEntityTooLarge:
		// 413 is server-side per http-contract.md (Atlassian's per-project
		// upload-size cap) — exit 5, not 3.
		return ErrorTypeServer
	case code >= 400 && code < 500:
		return ErrorTypeValidation
	default:
		return ErrorTypeServer
	}
}
