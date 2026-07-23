package jira

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/gechr/clog"
)

// Retry policy defaults. The cap matches Jira's documented practical
// Retry-After ceiling; four attempts (one try plus three retries) keeps a
// terminal command responsive while riding out a short burst pause.
const (
	defaultRetryMaxAttempts = 4
	defaultRetryBaseDelay   = 1 * time.Second
	defaultRetryMaxDelay    = 30 * time.Second
)

// retryTransport wraps a base RoundTripper and resends a safe request when
// Jira answers with a retryable status (429, or 503 carrying Retry-After).
// It honors Retry-After as a floor, otherwise backs off with full-jitter
// capped exponential delay, and stops once a further wait would exceed the
// budget (maxWait, itself clamped to the request's context deadline) or the
// attempt cap. It owns ONLY the resend loop: the response it returns — a
// success or the final retryable failure — flows up to Client.Do, which
// still parses rate headers and builds the typed APIError. Installing it on
// the shared *http.Client transport means every path, including the
// streaming attachment download, inherits retry without per-call plumbing.
//
// maxWait <= 0 disables retry: RoundTrip falls straight through to base, so
// the transport is inert until a caller opts in.
type retryTransport struct {
	base        http.RoundTripper
	maxWait     time.Duration
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	// safeToRetry reports whether a request may be auto-retried. It is the
	// negation of Client.isMutationRequest: reads and the whitelisted read
	// POSTs are safe; mutations are not, because Jira has no general
	// idempotency-key contract and a resent write could duplicate.
	safeToRetry func(*http.Request) bool
	sleep       func(ctx context.Context, d time.Duration) error
	jitter      func() float64
	now         func() time.Time
	// debug gates the per-backoff "say why" diagnostics. It mirrors the
	// Client's debug dumps: emit the reasoning only under --debug, never on
	// the normal path.
	debug bool
}

// RoundTrip implements http.RoundTripper. It falls straight through to the base
// transport unless retry is enabled and the request is both safe to replay and
// carries a rewindable body; otherwise it runs the resend loop, returning the
// first non-retryable response (success or the final retryable failure) for
// Client.Do to interpret. It never mutates the caller's request — each attempt
// rides a shallow clone with a freshly rewound body.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.maxWait <= 0 || t.safeToRetry == nil || !t.safeToRetry(req) || !replayableBody(req) {
		return t.base.RoundTrip(req)
	}

	deadline := t.now().Add(t.maxWait)
	if d, ok := req.Context().Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	for attempt := 0; ; attempt++ {
		// RoundTrip must not modify the caller's request, so each attempt
		// rides a shallow clone with its own freshly-rewound body. A bodyless
		// request (every GET) needs no clone. replayableBody guaranteed that a
		// request carrying a body also carries GetBody, so the rewind cannot
		// fail for lack of a source.
		attemptReq := req
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			attemptReq = req.Clone(req.Context())
			attemptReq.Body = body
		}

		resp, err := t.base.RoundTrip(attemptReq)
		if err != nil || !isRetryableStatus(resp) {
			// A transport error is not retried — it may be a cancellation
			// or a genuine network failure, neither a clean 429.
			if attempt > 0 && err == nil {
				t.logResolved(req, resp, attempt)
			}
			return resp, err
		}
		if attempt+1 >= t.maxAttempts {
			t.logGiveUp(req, resp, attempt, "retry attempt cap reached")
			return resp, nil // attempts exhausted; Client.Do builds the error
		}

		delay := t.backoff(attempt, retryAfterFromResponse(resp, t.now()))
		if !t.now().Add(delay).Before(deadline) {
			t.logGiveUp(req, resp, attempt, "retry budget would be exceeded before the backoff elapsed")
			return resp, nil // a further wait would exceed the budget; give up
		}

		t.logRetry(req, resp, attempt, delay)
		drainAndClose(resp.Body)
		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, err // context canceled mid-backoff
		}
	}
}

// backoff returns the next delay. Retry-After, when present, is the floor —
// plus a small jitter so a -p fan-out that all got the same Retry-After does
// not wake in lockstep. Without it, full jitter over a capped exponential
// window spreads concurrent retries.
func (t *retryTransport) backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter + time.Duration(t.jitter()*float64(time.Second))
	}
	window := t.baseDelay << attempt
	if window > t.maxDelay || window <= 0 {
		window = t.maxDelay
	}
	return time.Duration(t.jitter() * float64(window))
}

// logRetry emits the "say why" diagnostic for one backoff: the status that
// triggered it, the reason Jira gave, and how long we will wait. Gated on
// --debug, so the normal path stays silent.
func (t *retryTransport) logRetry(req *http.Request, resp *http.Response, attempt int, delay time.Duration) {
	if !t.debug {
		return
	}
	retryEvent(req, resp, attempt).
		Str("delay", delay.String()).
		Int("retry_after_s", retryAfterSeconds(resp.Header.Get("Retry-After"), t.now())).
		Msg("jira rate limit; retrying after backoff")
}

// logGiveUp records why the loop stopped retrying — the attempt cap or the
// wall-clock budget — so a stalled command stays explainable under --debug.
func (t *retryTransport) logGiveUp(req *http.Request, resp *http.Response, attempt int, why string) {
	if !t.debug {
		return
	}
	retryEvent(req, resp, attempt).Str("cause", why).Msg("jira rate limit; gave up")
}

// logResolved notes how a retried request finally settled once the status
// stopped being retryable: a 2xx means the rate limit cleared, anything else
// means we stopped retrying on a non-retryable status (e.g. a 429 burst that
// resolved to a 500). Either way the preceding 429s are explainable, and the
// message never claims success the status doesn't support.
func (t *retryTransport) logResolved(req *http.Request, resp *http.Response, attempt int) {
	if !t.debug {
		return
	}
	msg := "jira rate limit cleared after retry"
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg = "jira stopped retrying on a non-retryable status"
	}
	retryEvent(req, resp, attempt).Msg(msg)
}

// retryEvent seeds a debug event with the fields common to every retry log:
// method, sanitized path (no query, so JQL and keys never leak), the status,
// the 1-based attempt, and Jira's machine-readable RateLimit-Reason.
func retryEvent(req *http.Request, resp *http.Response, attempt int) *clog.Event {
	return debugLogger(req.Context()).Debug().
		Str("method", req.Method).
		Str("path", req.URL.EscapedPath()).
		Int("status", resp.StatusCode).
		Int("attempt", attempt+1).
		// The header value is server-controlled: obs-text (including a raw
		// C1 CSI byte) survives Go's header parsing, so it crosses the same
		// debug-sink sanitizer as response bodies.
		Str("reason", oneLineSnippet(resp.Header.Get("RateLimit-Reason"), 256))
}

// isRetryableStatus reports whether a response should be retried: a 429
// always, a 503 only when it carries Retry-After (an explicit "come back"
// signal rather than an opaque outage we should not hammer). Deliberately
// narrower than xhttp.IsRetryableStatus (408/429/any 5xx): auto-retrying a
// bare 5xx would hammer an outage, and the 503 rule reads the response
// header, which a code-only helper cannot express.
func isRetryableStatus(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusServiceUnavailable:
		return resp.Header.Get("Retry-After") != ""
	default:
		return false
	}
}

// retryAfterFromResponse extracts the Retry-After delay, reusing the shared
// parser so the HTTP-date and clock-skew handling stays in one place.
func retryAfterFromResponse(resp *http.Response, now time.Time) time.Duration {
	return time.Duration(retryAfterSeconds(resp.Header.Get("Retry-After"), now)) * time.Second
}

// replayableBody reports whether the request body can be rewound for a
// retry. A bodyless request (every GET) is trivially replayable; a request
// built via NewRequest carries a GetBody (bytes-backed), so the read POSTs
// replay. A streaming body without GetBody cannot, and those are mutations
// that are not auto-retried anyway.
func replayableBody(req *http.Request) bool {
	return req.Body == nil || req.GetBody != nil
}

// maxDrainBytes bounds the throwaway read of a discarded retryable response.
// A keep-alive connection only returns to the idle pool once its body reaches
// EOF, so we must drain fully to reuse it; the cap stops a pathologically
// large body from stalling the retry. A body over the cap simply forgoes
// reuse — correctness over the optimisation.
const maxDrainBytes = 64 << 10

// drainAndClose drains a discarded response body to EOF (within the cap) and
// closes it, so the underlying connection can be reused before the retry.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainBytes)) //nolint:errcheck // best-effort drain only affects connection reuse
	_ = body.Close()                                                //nolint:errcheck // retry proceeds on a new connection if close fails
}

// realSleep waits for d or the context, whichever comes first.
func realSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func defaultJitter() float64 { return rand.Float64() } //nolint:gosec // jitter, not crypto

// installRetryTransport wraps the client's transport with retryTransport,
// using the negation of isMutationRequest as the safety gate. It runs after
// options are applied so the budget and base URL are known. The wrapped
// client is cloned so a caller-supplied *http.Client is not mutated. With
// retryMaxWait == 0 (the default) the transport is inert, so wrapping is a
// no-op in behavior until the CLI opts in via --max-retry-wait.
func (c *Client) installRetryTransport() {
	if c.client == nil {
		return
	}
	base := c.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if _, already := base.(*retryTransport); already {
		return
	}
	sleep := c.retrySleep
	if sleep == nil {
		sleep = realSleep
	}
	jitter := c.retryJitter
	if jitter == nil {
		jitter = defaultJitter
	}
	cloned := *c.client
	cloned.Transport = &retryTransport{
		base:        base,
		maxWait:     c.retryMaxWait,
		maxAttempts: defaultRetryMaxAttempts,
		baseDelay:   defaultRetryBaseDelay,
		maxDelay:    defaultRetryMaxDelay,
		safeToRetry: func(req *http.Request) bool { return !c.isMutationRequest(req) },
		sleep:       sleep,
		jitter:      jitter,
		now:         time.Now,
		debug:       c.debug,
	}
	c.client = &cloned
}

// WithMaxRetryWait sets the rate-limit retry budget: the longest a single
// request will spend sleeping out 429 / Retry-After responses before it
// gives up and returns the typed error. Zero disables auto-retry. The CLI
// clamps the effective wait to the remaining --timeout deadline.
func WithMaxRetryWait(d time.Duration) Option {
	return func(c *Client) {
		if d < 0 {
			d = 0
		}
		c.retryMaxWait = d
	}
}

// withRetrySleep and withRetryJitter are unexported injection seams for
// deterministic retry tests (instant sleep, fixed jitter).
func withRetrySleep(fn func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.retrySleep = fn }
}

func withRetryJitter(fn func() float64) Option {
	return func(c *Client) { c.retryJitter = fn }
}
