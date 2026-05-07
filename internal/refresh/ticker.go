package refresh

import "time"

type Ticker struct {
	base     time.Duration
	interval time.Duration
	paused   bool
}

func NewTicker(seconds int) *Ticker {
	base := time.Duration(seconds) * time.Second
	if base <= 0 {
		base = 30 * time.Second
	}
	return &Ticker{base: base, interval: base}
}

func (t *Ticker) Interval() time.Duration {
	return t.interval
}

func (t *Ticker) Pause() {
	t.paused = true
}

func (t *Ticker) Resume() {
	t.paused = false
	t.interval = t.base
}

func (t *Ticker) Paused() bool {
	return t.paused
}

func (t *Ticker) BackoffRateLimit(retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = t.interval
	}
	t.interval += retryAfter
}
