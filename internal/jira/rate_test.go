package jira

import (
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

	rate := parseRate(res)
	if rate.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", rate.Remaining)
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

func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}
