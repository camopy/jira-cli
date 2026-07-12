package refresh

import "time"

// Ticker tracks a poll interval that can be paused and stretched under rate
// limiting. It holds a base interval (the steady-state cadence) and a current
// interval that backoff inflates and Resume restores. A Ticker carries no
// synchronization and is not safe for concurrent use.
type Ticker struct {
	base     time.Duration
	interval time.Duration
	paused   bool
}

// NewTicker returns a Ticker whose base and current interval are seconds. A
// non-positive seconds falls back to 30s, so a misconfigured cadence never
// becomes a busy loop.
func NewTicker(seconds int) *Ticker {
	base := time.Duration(seconds) * time.Second
	if base <= 0 {
		base = 30 * time.Second
	}
	return &Ticker{base: base, interval: base}
}

// Interval returns the current poll interval, including any accumulated
// rate-limit backoff.
func (t *Ticker) Interval() time.Duration {
	return t.interval
}

// Pause marks the ticker paused. It does not change the interval; callers check
// Paused before acting on a tick.
func (t *Ticker) Pause() {
	t.paused = true
}

// Resume clears the paused flag and resets the interval to the base cadence,
// discarding any backoff accumulated while rate-limited.
func (t *Ticker) Resume() {
	t.paused = false
	t.interval = t.base
}

// Paused reports whether the ticker is paused.
func (t *Ticker) Paused() bool {
	return t.paused
}

// BackoffRateLimit lengthens the interval by retryAfter after a rate-limit
// response, so polling eases off rather than hammering a throttled server. A
// non-positive retryAfter doubles the current interval as a fallback. Backoff
// accumulates until Resume resets it.
func (t *Ticker) BackoffRateLimit(retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = t.interval
	}
	t.interval += retryAfter
}
