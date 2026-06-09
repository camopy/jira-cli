package jira

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestRetryAfterSecondsDeltaAndDate(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"delta seconds", "30", 30},
		{"zero", "0", 0},
		{"negative clamps to zero", "-5", 0},
		{"empty", "", 0},
		{"whitespace", "  ", 0},
		{"junk", "soon", 0},
		{"http-date in future", now.Add(45 * time.Second).UTC().Format(http.TimeFormat), 45},
		{"http-date in past (skew) retries now", now.Add(-10 * time.Second).UTC().Format(http.TimeFormat), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryAfterSeconds(tc.raw, now); got != tc.want {
				t.Errorf("retryAfterSeconds(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRetryAfterSecondsRoundsUp(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	// An HTTP-date can only express whole seconds, so build the rounding case
	// through secondsUntil directly: 1.4s must round up to 2 so we never wake
	// before the server's deadline.
	if got := secondsUntil(now.Add(1400*time.Millisecond), now); got != 2 {
		t.Fatalf("secondsUntil(1.4s) = %d, want 2 (round up)", got)
	}
	if got := secondsUntil(now.Add(2*time.Second), now); got != 2 {
		t.Fatalf("secondsUntil(2s) = %d, want 2", got)
	}
	if got := secondsUntil(now.Add(-time.Second), now); got != 0 {
		t.Fatalf("secondsUntil(past) = %d, want 0", got)
	}
}

func TestParseResetHeader(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	epoch := now.Add(time.Minute).Unix()
	cases := []struct {
		name string
		raw  string
		want time.Time
	}{
		{"empty", "", time.Time{}},
		{"junk", "later", time.Time{}},
		{"http-date", now.Add(time.Minute).UTC().Format(http.TimeFormat), now.Add(time.Minute).Truncate(time.Second)},
		{"epoch seconds", itoa64(epoch), time.Unix(epoch, 0).UTC()},
		{"epoch boundary takes epoch branch", "1000000000", time.Unix(1_000_000_000, 0).UTC()},
		{"just below boundary is a delta", "999999999", now.Add(999_999_999 * time.Second).UTC()},
		{"small delta seconds", "20", now.Add(20 * time.Second).UTC()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHeader(tc.raw, now)
			if !got.Equal(tc.want) {
				t.Errorf("parseResetHeader(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseRatePopulatesAllFields(t *testing.T) {
	now := time.Now()
	res := &http.Response{Header: http.Header{}}
	res.Header.Set("X-RateLimit-Remaining", "3")
	res.Header.Set("Retry-After", "12")
	res.Header.Set("RateLimit-Reason", "jira-burst-based")
	res.Header.Set("X-RateLimit-Reset", "20")
	res.Header.Set("X-RateLimit-NearLimit", "true")

	rate := parseRate(res)
	if rate.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", rate.Remaining)
	}
	if !rate.NearLimit {
		t.Errorf("NearLimit = false, want true from X-RateLimit-NearLimit: true")
	}
	if rate.RetryAfterSeconds != 12 {
		t.Errorf("RetryAfterSeconds = %d, want 12", rate.RetryAfterSeconds)
	}
	if rate.Reason != "jira-burst-based" {
		t.Errorf("Reason = %q, want jira-burst-based", rate.Reason)
	}
	if rate.Reset.Before(now) {
		t.Errorf("Reset = %v, want roughly 20s ahead of %v", rate.Reset, now)
	}
}

func TestParseRateTolerantOfMissingHeaders(t *testing.T) {
	rate := parseRate(&http.Response{Header: http.Header{}})
	if rate.Remaining != 0 || rate.RetryAfterSeconds != 0 || rate.Reason != "" || !rate.Reset.IsZero() {
		t.Fatalf("empty headers should yield a zero Rate, got %+v", rate)
	}
}

func TestParseBoolHeader(t *testing.T) {
	cases := map[string]bool{
		"true": true, "TRUE": true, " true ": true,
		"false": false, "": false, "1": false, "yes": false, "junk": false,
	}
	for raw, want := range cases {
		if got := parseBoolHeader(raw); got != want {
			t.Errorf("parseBoolHeader(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestRateObserverFiresOnlyOnSuccess(t *testing.T) {
	var got []Rate
	observe := func(_ context.Context, r Rate) { got = append(got, r) }

	// Every response carries the near-limit header, but the observer must
	// fire only on the clean 2xx: a 404 returns before the success path, and
	// a 2xx with an unparseable body returns a server error before it too.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-NearLimit", "true")
		w.Header().Set("RateLimit-Reason", "jira-burst-based")
		switch r.URL.Path {
		case "/rest/api/3/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["nope"]}`))
		case "/rest/api/3/junk":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`<html>not json</html>`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	})
	client := newHTTPHandlerClient(handler, WithRateObserver(observe))

	okReq, _ := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/ok", nil)
	if _, err := client.Do(okReq, nil); err != nil {
		t.Fatalf("ok request: %v", err)
	}
	missReq, _ := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/missing", nil)
	if _, err := client.Do(missReq, nil); err == nil {
		t.Fatal("missing request should error")
	}
	// out!=nil forces a decode; the junk body makes Do return a server error,
	// so the observer must not fire for it.
	var sink map[string]any
	junkReq, _ := client.NewRequest(context.Background(), http.MethodGet, "rest/api/3/junk", nil)
	if _, err := client.Do(junkReq, &sink); err == nil {
		t.Fatal("junk-body request should error before the observer fires")
	}

	if len(got) != 1 {
		t.Fatalf("observer fired %d times, want 1 (clean success only)", len(got))
	}
	if !got[0].NearLimit || got[0].Reason != "jira-burst-based" {
		t.Fatalf("observer got %+v, want NearLimit=true reason=jira-burst-based", got[0])
	}
}

func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}
