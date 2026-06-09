package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeRT scripts a sequence of responses/errors and counts calls.
type fakeRT struct {
	calls int
	fn    func(req *http.Request, call int) (*http.Response, error)
}

func (f *fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	call := f.calls
	f.calls++
	return f.fn(req, call)
}

func mkResp(status int, header map[string]string, body string) *http.Response {
	h := http.Header{}
	for k, v := range header {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

// closeBody closes the final response a test holds. mkResp bodies are
// NopClosers, so this only quiets the linters; it does not disturb state.
func closeBody(resp *http.Response) {
	if resp != nil {
		_ = resp.Body.Close()
	}
}

// newRT builds a retry transport with instant, recorded sleeps and fixed
// jitter so delays are deterministic.
func newRT(base http.RoundTripper, maxWait time.Duration) (*retryTransport, *[]time.Duration) {
	slept := []time.Duration{}
	t := &retryTransport{
		base:        base,
		maxWait:     maxWait,
		maxAttempts: defaultRetryMaxAttempts,
		baseDelay:   defaultRetryBaseDelay,
		maxDelay:    defaultRetryMaxDelay,
		safeToRetry: func(*http.Request) bool { return true },
		sleep: func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		},
		jitter: func() float64 { return 0.5 },
		now:    time.Now,
	}
	return t, &slept
}

func getReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://jira.example/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestRetryRetriesUntilSuccess(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, call int) (*http.Response, error) {
		if call < 2 {
			return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "2"}, "limited"), nil
		}
		return mkResp(http.StatusOK, nil, "ok"), nil
	}}
	rt, slept := newRT(base, 30*time.Second)
	resp, err := rt.RoundTrip(getReq(t))
	defer closeBody(resp)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %v err=%v", resp.StatusCode, err)
	}
	if base.calls != 3 {
		t.Fatalf("want 3 calls (2 retries), got %d", base.calls)
	}
	// Retry-After floor (2s) plus fixed 0.5s jitter.
	for _, d := range *slept {
		if d < 2*time.Second {
			t.Fatalf("delay %v below Retry-After floor", d)
		}
	}
}

func TestRetry429WithoutRetryAfterUsesJitteredBackoff(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, nil, "limited"), nil
	}}
	rt, slept := newRT(base, 30*time.Second)
	resp, _ := rt.RoundTrip(getReq(t))
	defer closeBody(resp)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want final 429, got %d", resp.StatusCode)
	}
	if base.calls != defaultRetryMaxAttempts {
		t.Fatalf("want %d attempts, got %d", defaultRetryMaxAttempts, base.calls)
	}
	// jitter=0.5 over windows 1s,2s,4s → 0.5s,1s,2s, all within the cap.
	for i, d := range *slept {
		if d <= 0 || d > defaultRetryMaxDelay {
			t.Fatalf("delay[%d]=%v out of bounds", i, d)
		}
	}
}

func TestRetry503OnlyWithRetryAfter(t *testing.T) {
	// 503 WITH Retry-After is retried.
	withRA := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusServiceUnavailable, map[string]string{"Retry-After": "1"}, "down"), nil
	}}
	rt, _ := newRT(withRA, 30*time.Second)
	resp, _ := rt.RoundTrip(getReq(t))
	closeBody(resp)
	if withRA.calls != defaultRetryMaxAttempts {
		t.Fatalf("503+Retry-After should retry: got %d calls", withRA.calls)
	}
	// 503 WITHOUT Retry-After is not retried.
	noRA := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusServiceUnavailable, nil, "down"), nil
	}}
	rt2, _ := newRT(noRA, 30*time.Second)
	resp2, _ := rt2.RoundTrip(getReq(t))
	closeBody(resp2)
	if noRA.calls != 1 {
		t.Fatalf("503 without Retry-After must not retry: got %d calls", noRA.calls)
	}
}

func TestRetryMalformedRetryAfterFallsBackToJitter(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "soon"}, "x"), nil
	}}
	rt, slept := newRT(base, 30*time.Second)
	resp, err := rt.RoundTrip(getReq(t)) // must not panic
	defer closeBody(resp)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(*slept) == 0 {
		t.Fatal("expected jittered backoff despite malformed Retry-After")
	}
}

func TestRetryContextCancelMidBackoff(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
	}}
	rt, _ := newRT(base, 30*time.Second)
	rt.sleep = func(_ context.Context, _ time.Duration) error { return context.Canceled }
	resp, err := rt.RoundTrip(getReq(t))
	defer closeBody(resp)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got resp=%v err=%v", resp, err)
	}
	if resp != nil {
		t.Fatalf("want nil response on cancel, got %v", resp.StatusCode)
	}
}

func TestRetryGivesUpOverBudget(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "10"}, "x"), nil
	}}
	rt, slept := newRT(base, 1*time.Second) // budget 1s, Retry-After 10s
	resp, _ := rt.RoundTrip(getReq(t))
	defer closeBody(resp)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want final 429, got %d", resp.StatusCode)
	}
	if base.calls != 1 || len(*slept) != 0 {
		t.Fatalf("over-budget must give up without sleeping: calls=%d slept=%d", base.calls, len(*slept))
	}
}

func TestRetryClampsBudgetToContextDeadline(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "10"}, "x"), nil
	}}
	rt, slept := newRT(base, 30*time.Second) // generous wall-clock budget
	// A context deadline sooner than the budget AND sooner than Retry-After
	// must cap the wait: the first backoff (~10s) already overruns the 2s
	// deadline, so the transport gives up after the first attempt.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Second))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://jira.example/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := rt.RoundTrip(req)
	defer closeBody(resp)
	if base.calls != 1 || len(*slept) != 0 {
		t.Fatalf("ctx deadline (2s) must cap the 30s budget below the 10s Retry-After: calls=%d slept=%d", base.calls, len(*slept))
	}
}

func TestRetryDisabledWhenMaxWaitZero(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
	}}
	rt, _ := newRT(base, 0)
	resp, _ := rt.RoundTrip(getReq(t))
	defer closeBody(resp)
	if base.calls != 1 || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("maxWait=0 must pass through: calls=%d", base.calls)
	}
}

func TestRetryNotAppliedToUnsafeRequests(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
	}}
	rt, _ := newRT(base, 30*time.Second)
	rt.safeToRetry = func(*http.Request) bool { return false } // mutation
	resp, _ := rt.RoundTrip(getReq(t))
	closeBody(resp)
	if base.calls != 1 {
		t.Fatalf("unsafe request must not retry: got %d calls", base.calls)
	}
}

func TestRetryReplaysRequestBody(t *testing.T) {
	got := []string{}
	base := &fakeRT{fn: func(req *http.Request, call int) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		got = append(got, string(b))
		if call < 1 {
			return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
		}
		return mkResp(http.StatusOK, nil, "ok"), nil
	}}
	// A bytes-backed POST has GetBody set automatically.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://jira.example/search/jql", strings.NewReader(`{"jql":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	rt, _ := newRT(base, 30*time.Second)
	resp, err := rt.RoundTrip(req)
	defer closeBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != got[1] || got[0] != `{"jql":"x"}` {
		t.Fatalf("body not replayed identically across retries: %v", got)
	}
}

// recordingBody tracks whether it was drained to EOF and closed.
type recordingBody struct {
	r      io.Reader
	eof    bool
	closed bool
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

func (b *recordingBody) Close() error {
	b.closed = true
	return nil
}

func TestDrainAndCloseReadsBodyToEOFForReuse(t *testing.T) {
	// A body under the drain cap must be read to EOF (so the keep-alive
	// connection can return to the pool) and then closed. Under the old
	// 4 KiB CopyN this 8 KiB body would stop short of EOF.
	body := &recordingBody{r: strings.NewReader(strings.Repeat("x", 8<<10))}
	drainAndClose(body)
	if !body.eof {
		t.Fatal("body must be drained to EOF so the connection can be reused")
	}
	if !body.closed {
		t.Fatal("body must be closed")
	}
}

func TestRealSleepReturnsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := realSleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRealSleepCompletesForShortDelay(t *testing.T) {
	if err := realSleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("want nil after sleeping out the delay, got %v", err)
	}
}

func TestRetryTransportInstalledButInertByDefault(t *testing.T) {
	// A default client wraps the transport, but with no budget it must not
	// change behavior: one call, the 429 surfaces unchanged.
	base := &fakeRT{fn: func(_ *http.Request, _ int) (*http.Response, error) {
		return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
	}}
	c, err := NewClientE(
		WithBaseURL("https://jira.example/"),
		WithHTTPClient(&http.Client{Transport: base}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.client.Transport.(*retryTransport); !ok {
		t.Fatal("retry transport should be installed on the client")
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://jira.example/x", nil)
	resp, _ := c.client.Do(req)
	defer closeBody(resp)
	if base.calls != 1 || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("default (no budget) must be inert: calls=%d", base.calls)
	}
}

func TestRetryEnabledThroughClientOption(t *testing.T) {
	base := &fakeRT{fn: func(_ *http.Request, call int) (*http.Response, error) {
		if call < 1 {
			return mkResp(http.StatusTooManyRequests, map[string]string{"Retry-After": "1"}, "x"), nil
		}
		return mkResp(http.StatusOK, nil, "ok"), nil
	}}
	c, err := NewClientE(
		WithBaseURL("https://jira.example/"),
		WithHTTPClient(&http.Client{Transport: base}),
		WithMaxRetryWait(30*time.Second),
		withRetrySleep(func(context.Context, time.Duration) error { return nil }),
		withRetryJitter(func() float64 { return 0 }),
	)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://jira.example/x", nil)
	resp, err := c.client.Do(req)
	defer closeBody(resp)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("want retried 200, got %v err=%v", resp.StatusCode, err)
	}
	if base.calls != 2 {
		t.Fatalf("want 2 calls (1 retry), got %d", base.calls)
	}
}
