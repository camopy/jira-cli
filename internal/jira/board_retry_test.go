package jira

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// TestBoardListAllRidesOutTransientRateLimit proves the retry transport
// resends a mid-pagination 429 underneath ListAll: page 2 fails once with a
// 429+Retry-After, the transport retries it, and the drain completes with no
// RateLimitHit. This is the "retry the page fetch" behavior — owned by the
// shared transport, so ListAll needs no bespoke retry logic of its own.
func TestBoardListAllRidesOutTransientRateLimit(t *testing.T) {
	var page2 atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("startAt") {
		case "", "0":
			_, _ = w.Write([]byte(`{"maxResults":1,"startAt":0,"isLast":false,"values":[{"id":1,"name":"Board 1","type":"scrum"}]}`))
		default:
			// Page 2 is rate-limited once, then succeeds on the retry.
			if page2.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errorMessages":["rate limited"]}`))
				return
			}
			_, _ = w.Write([]byte(`{"maxResults":1,"startAt":1,"isLast":true,"values":[{"id":2,"name":"Board 2","type":"scrum"}]}`))
		}
	})
	client := newHTTPHandlerClient(
		handler,
		WithMaxRetryWait(30*time.Second),
		withRetrySleep(func(context.Context, time.Duration) error { return nil }),
		withRetryJitter(func() float64 { return 0 }),
	)
	out, err := NewBoardService(client).ListAll(context.Background(), BoardDrainOptions{PageSize: 1, Unbounded: true})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if out.RateLimitHit != nil {
		t.Fatalf("a transient 429 must be retried, not surfaced: %+v", out.RateLimitHit)
	}
	if out.Truncated {
		t.Fatalf("the drain should complete, got truncated=%q", out.TruncatedReason)
	}
	if len(out.Boards) != 2 {
		t.Fatalf("drained %d boards, want 2 (both pages)", len(out.Boards))
	}
	if page2.Load() != 2 {
		t.Fatalf("page 2 should be fetched twice (429 then retry), got %d", page2.Load())
	}
}

// TestBoardListAllSurfacesRateLimitAfterBudget proves the other side: when the
// budget cannot absorb the 429 (Retry-After exceeds it), ListAll still surfaces
// the partial drain as a rate-limit truncation rather than spinning forever.
func TestBoardListAllSurfacesRateLimitAfterBudget(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("startAt") {
		case "", "0":
			_, _ = w.Write([]byte(`{"maxResults":1,"startAt":0,"isLast":false,"values":[{"id":1,"name":"Board 1","type":"scrum"}]}`))
		default:
			// Retry-After far exceeds the budget → give up on the first attempt.
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"errorMessages":["rate limited"]}`))
		}
	})
	client := newHTTPHandlerClient(
		handler,
		WithMaxRetryWait(5*time.Second),
		withRetrySleep(func(context.Context, time.Duration) error { return nil }),
		withRetryJitter(func() float64 { return 0 }),
	)
	out, err := NewBoardService(client).ListAll(context.Background(), BoardDrainOptions{PageSize: 1, Unbounded: true})
	if err != nil {
		t.Fatalf("ListAll should preserve the partial drain, got err: %v", err)
	}
	if out.RateLimitHit == nil || out.TruncatedReason != "rate_limit" {
		t.Fatalf("expected a rate-limit truncation, got truncated=%v reason=%q hit=%v", out.Truncated, out.TruncatedReason, out.RateLimitHit)
	}
	if len(out.Boards) != 1 {
		t.Fatalf("the page-1 board must be preserved, got %d boards", len(out.Boards))
	}
}
