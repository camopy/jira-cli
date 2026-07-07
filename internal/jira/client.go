package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gechr/clog"
	xmaps "github.com/gechr/x/maps"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/errtax"
)

const (
	defaultBaseURL       = "https://api.atlassian.com/"
	defaultHTTPTimeout   = 30 * time.Second
	defaultMaxIdleConns  = 100
	maxIdleConnsPerHost  = 16
	maxErrorBodyBytes    = 4096
	maxResponseBodyBytes = 16 << 20
)

// ErrorType aliases the shared taxonomy type so jira's status
// classification and the envelope mapper speak one enum.
type ErrorType = errtax.Type

// The taxonomy types, re-exported under jira's historical names.
const (
	ErrorTypeAuth       = errtax.TypeAuth
	ErrorTypeNotFound   = errtax.TypeNotFound
	ErrorTypeValidation = errtax.TypeValidation
	ErrorTypeRateLimit  = errtax.TypeRateLimit
	ErrorTypeServer     = errtax.TypeServer
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
	// RateLimitRemaining is the X-RateLimit-Remaining header value when
	// Jira sends it. Zero can mean either absent or exhausted; callers use
	// it as diagnostic context only.
	RateLimitRemaining int
	// UpstreamRequestID is Jira's own trace id for the failed exchange,
	// read from the Atl-Traceid (preferred) or X-ARequestId response
	// header. It is the value Atlassian support can correlate a failure
	// against; the envelope's request_id is a local value with no
	// server-side meaning. Empty when neither header was present.
	UpstreamRequestID string
	// Cause preserves transport/body/JSON failures for errors.Is/As while
	// keeping the public APIError message stable.
	Cause error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira: %s (%d): %s", e.Type, e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Cause
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

// displayMessage builds the concise human message for an error response.
// It prefers Jira's parsed errorMessages[], then the field errors map
// (sorted for stable output), and only falls back to the raw redacted
// body when the response was not a recognized ErrorCollection — an HTML
// maintenance page or an empty body. The full body still lives in
// resp.RawBody and the structured fields stay on the APIError, so this
// keeps the displayed line clean without dropping machine detail.
func displayMessage(ec errorCollection, rawBody string) string {
	if len(ec.ErrorMessages) > 0 {
		return strings.Join(ec.ErrorMessages, "; ")
	}
	if len(ec.Errors) > 0 {
		parts := make([]string, 0, len(ec.Errors))
		for k, v := range xmaps.Sorted(ec.Errors) {
			parts = append(parts, k+": "+v)
		}
		return strings.Join(parts, "; ")
	}
	return strings.TrimSpace(rawBody)
}

type Client struct {
	client     *http.Client
	baseURL    *url.URL
	basicEmail string
	basicToken string
	debug      bool
	readOnly   bool
	dryRun     bool
	initErr    error
	// retryMaxWait is the rate-limit retry budget. Zero (the default)
	// disables auto-retry; the CLI sets it from --max-retry-wait. retrySleep
	// and retryJitter are injection seams for deterministic tests.
	retryMaxWait time.Duration
	retrySleep   func(context.Context, time.Duration) error
	retryJitter  func() float64
	// rateObserver, when set, is called with the parsed Rate of every
	// successful response. The CLI uses it to surface a near-limit warning
	// without every command inspecting the response itself.
	rateObserver RateObserver
}

// RateObserver receives the rate-limit state of a successful response. It runs
// on the request's goroutine — possibly one of several under -p fan-out — so
// an implementation must be safe for concurrent calls.
type RateObserver func(context.Context, Rate)

type Option func(*Client)

func NewClient(opts ...Option) *Client {
	c, _ := newClient(opts...)
	return c
}

func NewClientE(opts ...Option) (*Client, error) {
	c, err := newClient(opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newClient(opts ...Option) (*Client, error) {
	u, _ := url.Parse(defaultBaseURL)
	c := &Client{client: defaultHTTPClient(), baseURL: u}
	for _, opt := range opts {
		opt(c)
	}
	if c.initErr != nil {
		return c, c.initErr
	}
	c.installRetryTransport()
	return c, nil
}

func defaultHTTPClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{
			Timeout:   defaultHTTPTimeout,
			Transport: http.DefaultTransport,
		}
	}
	transport = transport.Clone()
	transport.MaxIdleConns = defaultMaxIdleConns
	transport.MaxIdleConnsPerHost = maxIdleConnsPerHost
	return &http.Client{
		Timeout:   defaultHTTPTimeout,
		Transport: transport,
	}
}

func (c *Client) setInitErr(err error) {
	if err != nil && c.initErr == nil {
		c.initErr = err
	}
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h == nil {
			c.setInitErr(fmt.Errorf("jira client: http client must not be nil"))
			return
		}
		c.client = h
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
		u, err := parseClientBaseURL(raw)
		if err != nil {
			c.setInitErr(err)
			return
		}
		c.baseURL = u
	}
}

func parseClientBaseURL(raw string) (*url.URL, error) {
	if xstrings.IsBlank(raw) {
		return nil, fmt.Errorf("jira client: base URL is required")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("jira client: base URL is malformed: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("jira client: base URL must use http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("jira client: base URL must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("jira client: base URL must not include query or fragment")
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u, nil
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

// WithRateObserver registers a callback invoked with the rate-limit state of
// every successful response. The CLI uses it to raise a near-limit warning;
// it is a no-op observability seam, so a nil observer (the default) changes
// nothing.
func WithRateObserver(fn RateObserver) Option {
	return func(c *Client) {
		c.rateObserver = fn
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

// WithDryRun causes Do to refuse any state-changing HTTP method
// (POST / PUT / PATCH / DELETE) so a --dry-run invocation cannot mutate
// Jira. Reads still pass through, so the same client can serve a
// dry-run preview that renders live data. This is the service-level
// safety net behind the command-layer dry-run branches: even a command
// path that forgets to gate a submission is stopped here.
func WithDryRun(dryRun bool) Option {
	return func(c *Client) {
		c.dryRun = dryRun
	}
}

func (c *Client) NewRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := c.NewRawRequest(ctx, method, path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) NewRawRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	u, err := c.requestURL(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	c.SignRequest(req)
	return req, nil
}

func (c *Client) requestURL(path string) (*url.URL, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	cleanPath, err := validateRequestPath(path)
	if err != nil {
		return nil, err
	}
	rel, err := url.Parse(cleanPath)
	if err != nil {
		return nil, err
	}
	u := c.baseURL.ResolveReference(rel)
	return u, nil
}

func (c *Client) validate() error {
	if c == nil {
		return fmt.Errorf("jira client is nil")
	}
	if c.initErr != nil {
		return c.initErr
	}
	if c.baseURL == nil {
		return fmt.Errorf("jira client base URL is not configured")
	}
	if c.client == nil {
		return fmt.Errorf("jira client HTTP client is not configured")
	}
	return nil
}

func validateRequestPath(path string) (string, error) {
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("jira request path contains control characters")
		}
	}
	if strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("jira request path must be relative")
	}
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", nil
	}
	rawPath, _, _ := strings.Cut(trimmed, "?")
	rawPath, _, _ = strings.Cut(rawPath, "#")
	for segment := range strings.SplitSeq(rawPath, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("jira request path contains dot segment %q", segment)
		}
	}
	rel, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if rel.IsAbs() || rel.Host != "" {
		return "", fmt.Errorf("jira request path must be relative")
	}
	if rel.Fragment != "" {
		return "", fmt.Errorf("jira request path must not include a fragment")
	}
	return trimmed, nil
}

// SignRequest applies the active profile's Jira Cloud auth to req: HTTP
// Basic with the account email and API token. Services that build requests
// outside NewRequest (multipart upload, raw-body posts, streaming download)
// MUST call this rather than re-implementing the basic-auth selection inline
// so the auth surface stays consistent.
func (c *Client) SignRequest(req *http.Request) {
	// Only attach Basic auth when we have a complete Jira Cloud credential
	// pair (account email + API token). Sending a half-pair produces a
	// malformed header that Jira rejects with a confusing 401; skipping it
	// surfaces a cleaner "no credential" failure instead.
	if c.basicEmail != "" && c.basicToken != "" {
		req.SetBasicAuth(c.basicEmail, c.basicToken)
	}
}

// BaseURL exposes the resolved base URL so services that need to build
// fully-qualified URLs without going through NewRequest (multipart,
// streaming download) can ResolveReference against the same root the
// rest of the package uses.
func (c *Client) BaseURL() *url.URL {
	if c == nil || c.baseURL == nil {
		return nil
	}
	u := *c.baseURL
	return &u
}

func (c *Client) Do(req *http.Request, out any) (*Response, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("jira request is nil")
	}
	if req.URL == nil {
		return nil, fmt.Errorf("jira request URL is nil")
	}
	if err := c.validateRequestTarget(req); err != nil {
		return nil, err
	}
	if c.readOnly && c.isMutationRequest(req) {
		return nil, &ReadOnlyError{Method: req.Method, Path: req.URL.Path}
	}
	if c.dryRun && c.isMutationRequest(req) {
		return nil, &DryRunBlockedError{Method: req.Method, Path: req.URL.Path}
	}
	ctx := req.Context()
	if c.debug {
		c.dumpRequest(ctx, req)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request %s %s: %w", req.Method, req.URL.EscapedPath(), err)
	}
	defer func() { _ = res.Body.Close() }()
	if c.debug {
		c.dumpResponseHead(ctx, res)
	}
	resp := &Response{Response: res, Rate: parseRate(res)}
	bodyLimit := maxResponseBodyBytes
	if res.StatusCode < 200 || res.StatusCode > 299 {
		bodyLimit = maxErrorBodyBytes
	}
	body, truncated, readErr := readLimitedBody(res.Body, bodyLimit)
	if c.debug {
		c.dumpResponseBody(ctx, body)
	}
	if readErr != nil {
		// A network blip mid-read MUST surface as a typed server error
		// rather than degrade into a JSON-decode failure downstream.
		// Caller maps server → exit 5.
		return resp, &APIError{
			StatusCode:        res.StatusCode,
			Type:              ErrorTypeServer,
			Message:           "read response body: " + readErr.Error(),
			UpstreamRequestID: upstreamRequestID(res),
			Cause:             readErr,
		}
	}
	if truncated && res.StatusCode >= 200 && res.StatusCode <= 299 {
		return resp, &APIError{
			StatusCode:        res.StatusCode,
			Type:              ErrorTypeServer,
			Message:           fmt.Sprintf("response body exceeded %d bytes", maxResponseBodyBytes),
			UpstreamRequestID: upstreamRequestID(res),
		}
	}
	if len(body) > 0 {
		resp.RawBody = append(resp.RawBody[:0], body...)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msgBody := redactSensitiveBytes(body)
		ec := parseErrorCollection(msgBody)
		apiErr := &APIError{
			StatusCode:         res.StatusCode,
			Type:               ClassifyStatus(res.StatusCode),
			Message:            displayMessage(ec, string(msgBody)),
			ErrorMessages:      ec.ErrorMessages,
			FieldErrors:        ec.Errors,
			UpstreamStatus:     ec.Status,
			RetryAfterSeconds:  resp.Rate.RetryAfterSeconds,
			RateLimitRemaining: resp.Rate.Remaining,
			UpstreamRequestID:  upstreamRequestID(res),
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
			snippet := redactSensitiveBytes(body)
			if len(snippet) > maxErrorBodyBytes {
				snippet = snippet[:maxErrorBodyBytes]
			}
			return resp, &APIError{
				StatusCode:        res.StatusCode,
				Type:              ErrorTypeServer,
				Message:           "response body is not valid JSON: " + err.Error() + "; body prefix: " + strings.TrimSpace(string(snippet)),
				UpstreamRequestID: upstreamRequestID(res),
				Cause:             err,
			}
		}
	}
	// Fully-successful (2xx, body decoded) response: hand its rate state to
	// the observer so a near-limit signal can surface as a warning. Every
	// error — non-2xx, truncation, read failure, unparseable body — has
	// returned above, so the observer only ever sees a clean success.
	if c.rateObserver != nil {
		c.rateObserver(ctx, resp.Rate)
	}
	return resp, nil
}

func readLimitedBody(body io.Reader, limit int) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return data, false, err
	}
	if len(data) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

func (c *Client) validateRequestTarget(req *http.Request) error {
	if req.URL.Scheme != c.baseURL.Scheme || req.URL.Host != c.baseURL.Host {
		return fmt.Errorf("jira request target %s://%s does not match client base %s://%s", req.URL.Scheme, req.URL.Host, c.baseURL.Scheme, c.baseURL.Host)
	}
	if _, ok := c.relativeEscapedPath(req); !ok {
		return fmt.Errorf("jira request path %q is outside client base path %q", req.URL.EscapedPath(), c.baseURL.EscapedPath())
	}
	return nil
}

// dumpRequest writes the outbound request through the context logger.
// The Authorization header is redacted to "REDACTED" so token material
// never reaches the log. Body is reset after read so the http.Client
// still sees it on the actual send.
func (c *Client) dumpRequest(ctx context.Context, req *http.Request) {
	headers := redactHeaders(req.Header)
	bodyText := ""
	if req.Body != nil && req.GetBody != nil {
		if isDebuggableBody(req.Header.Get("Content-Type")) {
			// req.Body has already been used to build the request; rewind
			// via GetBody for the dump. http.NewRequestWithContext sets
			// GetBody when the body is a *bytes.Reader.
			body, err := req.GetBody()
			if err == nil && body != nil {
				data, _ := io.ReadAll(body)
				_ = body.Close()
				bodyText = string(redactSensitiveBytes(data))
			}
		} else {
			bodyText = "(redacted non-json body)"
		}
	}
	event := debugLogger(ctx).Debug().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Dict("headers", debugHeaderDict(headers))
	addDebugBody(event, bodyText, 1024).Msg("jira request")
}

func isDebuggableBody(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

// dumpResponseHead writes status + headers through the context logger.
func (c *Client) dumpResponseHead(ctx context.Context, res *http.Response) {
	debugLogger(ctx).Debug().
		Int("status_code", res.StatusCode).
		Str("status", res.Status).
		Dict("headers", debugHeaderDict(redactHeaders(res.Header))).
		Msg("jira response")
}

// dumpResponseBody writes the response body through the context logger.
func (c *Client) dumpResponseBody(ctx context.Context, body []byte) {
	if len(body) == 0 {
		addDebugBody(debugLogger(ctx).Debug(), "(empty)", 2048).Msg("jira response body")
		return
	}
	addDebugBody(debugLogger(ctx).Debug(), string(redactSensitiveBytes(body)), 2048).Msg("jira response body")
}

func debugLogger(ctx context.Context) *clog.Logger {
	logger := clog.Ctx(ctx)
	if logger != clog.Default || logger.LevelEnabled(clog.LevelDebug) {
		return logger
	}

	// Preserve direct use of WithDebug(true) outside the Cobra command
	// path. CLI execution normally configures clog.Default to stderr.
	fallback := clog.New(clog.NewOutput(os.Stderr, clog.ColorAuto))
	fallback.SetLevel(clog.LevelDebug)
	return fallback
}

func debugHeaderDict(headers http.Header) *clog.Event {
	dict := clog.Dict()
	for k, values := range xmaps.Sorted(headers) {
		dict.Str(k, strings.Join(values, ", "))
	}
	return dict
}

func addDebugBody(event *clog.Event, bodyText string, maxLen int) *clog.Event {
	if bodyText == "" {
		return event
	}
	return event.Str("body", oneLineSnippet(bodyText, maxLen))
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
			out[k] = append([]string(nil), v...)
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

func redactSensitiveBytes(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return []byte(redactSensitiveText(string(body)))
	}
	redacted, err := json.Marshal(redactJSONValue("", value))
	if err != nil {
		return []byte(redactSensitiveText(string(body)))
	}
	return redacted
}

func redactJSONValue(key string, value any) any {
	if sensitiveJSONKey(key) {
		return "REDACTED"
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = redactJSONValue(k, item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactJSONValue("", item)
		}
		return out
	default:
		return value
	}
}

func sensitiveJSONKey(key string) bool {
	lower := strings.ToLower(key)
	for _, token := range []string{"authorization", "cookie", "password", "secret", "token", "api_key", "apikey", "private_key"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func redactSensitiveText(text string) string {
	for _, marker := range []string{"authorization", "password", "secret", "token", "api_key", "apikey", "private_key"} {
		if strings.Contains(strings.ToLower(text), marker) {
			return "REDACTED"
		}
	}
	return text
}

// upstreamRequestID extracts Jira's own trace id from a response.
// Atlassian emits it as Atl-Traceid (preferred; the id support quotes)
// with X-ARequestId as the older per-node spelling. Empty when neither
// header is present.
func upstreamRequestID(res *http.Response) string {
	if res == nil {
		return ""
	}
	if v := strings.TrimSpace(res.Header.Get("Atl-Traceid")); v != "" {
		return v
	}
	return strings.TrimSpace(res.Header.Get("X-ARequestId"))
}

func parseRate(res *http.Response) Rate {
	remaining, _ := strconv.Atoi(res.Header.Get("X-RateLimit-Remaining"))
	now := time.Now()
	return Rate{
		Remaining:         remaining,
		RetryAfterSeconds: retryAfterSeconds(res.Header.Get("Retry-After"), now),
		Reset:             parseResetHeader(res.Header.Get("X-RateLimit-Reset"), now),
		Reason:            strings.TrimSpace(res.Header.Get("RateLimit-Reason")),
		NearLimit:         parseBoolHeader(res.Header.Get("X-RateLimit-NearLimit")),
	}
}

// parseBoolHeader reads a boolean-ish header. Jira sends X-RateLimit-NearLimit
// as "true"/"false"; anything else (absent, blank, junk) reads false so a
// missing or malformed header never raises a spurious warning.
func parseBoolHeader(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

// retryAfterSeconds parses a Retry-After value. RFC 9110 allows either
// delta-seconds or an HTTP-date; we accept both. A past HTTP-date (server
// clock skew) yields 0 — safe to retry now. Absent, negative, or
// unparseable values yield 0 rather than erroring, so a malformed header
// never blocks a request.
func retryAfterSeconds(raw string, now time.Time) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0
		}
		return secs
	}
	if t, err := http.ParseTime(raw); err == nil {
		return secondsUntil(t, now)
	}
	return 0
}

// parseResetHeader parses X-RateLimit-Reset, tolerating the forms Jira and
// proxies emit: an HTTP-date, epoch seconds, or a small delta-seconds. An
// absent or unparseable value yields the zero time. Observability only —
// the retry decision does not depend on it.
func parseResetHeader(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := http.ParseTime(raw); err == nil {
		return t
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		// A value in the Unix-epoch range (>= 2001-09-09) is an absolute
		// timestamp; a smaller one is seconds from now. Deltas are realistically
		// minutes-to-hours, so the gap to a real epoch (~1.7e9) is enormous —
		// the boundary itself is a valid epoch and must take the epoch branch.
		if n >= 1_000_000_000 {
			return time.Unix(n, 0).UTC()
		}
		if n > 0 {
			return now.Add(time.Duration(n) * time.Second).UTC()
		}
	}
	return time.Time{}
}

// secondsUntil returns the whole seconds from now until t, rounded up so a
// caller never wakes before the deadline. Non-positive durations yield 0.
func secondsUntil(t, now time.Time) int {
	d := t.Sub(now)
	if d <= 0 {
		return 0
	}
	secs := int(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	return secs
}

// isMutationRequest decides whether a request actually changes server
// state. Method alone isn't enough — Jira's /search/jql endpoint uses
// POST so callers can send large JQL bodies, but it's a pure read.
func (c *Client) isMutationRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	// Whitelist Jira read endpoints that happen to use POST.
	relativePath, ok := c.relativeEscapedPath(req)
	if req.Method == http.MethodPost && ok && relativePath == "/rest/api/3/search/jql" {
		return false
	}
	return true
}

func (c *Client) relativeEscapedPath(req *http.Request) (string, bool) {
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	basePath := c.baseURL.EscapedPath()
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	if basePath == "/" {
		return path, true
	}
	if !strings.HasPrefix(path, basePath) {
		return "", false
	}
	return "/" + strings.TrimPrefix(path, basePath), true
}

// CodeForStatus derives the taxonomy code for an HTTP status. It moves in
// lockstep with ClassifyStatus and the errtax registry: a status added here
// MUST carry a registry row (the taxonomy guard enumerates the pairing) —
// a jira_* code with no hint is the exact gap that guard exists to catch.
// An unmapped status falls back to its classified type's catch-all code.
func CodeForStatus(status int) errtax.Code {
	switch status {
	case http.StatusUnauthorized:
		return errtax.CodeJiraUnauthorized
	case http.StatusForbidden:
		return errtax.CodeJiraForbidden
	case http.StatusNotFound:
		return errtax.CodeJiraNotFound
	case http.StatusConflict:
		return errtax.CodeJiraConflict
	case http.StatusGone:
		return errtax.CodeJiraGone
	case http.StatusTooManyRequests:
		return errtax.CodeJiraRateLimited
	case http.StatusBadRequest:
		return errtax.CodeJiraBadRequest
	default:
		if status >= http.StatusInternalServerError {
			return errtax.CodeJiraServerError
		}
		return errtax.DefaultCode(ClassifyStatus(status))
	}
}

// Code classifies the API failure by its HTTP status.
func (e *APIError) Code() errtax.Code { return CodeForStatus(e.StatusCode) }

var _ errtax.Coded = (*APIError)(nil)

// ClassifyStatus reports the error type for an HTTP status. It stays an
// independent status→type switch (never derived from CodeForStatus plus the
// registry) so the taxonomy lockstep test compares two genuine sources.
func ClassifyStatus(code int) ErrorType {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrorTypeAuth
	case code == http.StatusNotFound:
		return ErrorTypeNotFound
	case code == http.StatusGone:
		// 410 is a permanently deleted resource: not-found (exit 2) tells an
		// agent to fix the reference, where validation (exit 3) would
		// suggest correcting the request.
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
