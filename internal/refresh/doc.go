// Package refresh models the cadence of a periodic poll: the current interval,
// pause and resume, and additive backoff when the server reports a rate limit.
// It computes durations only — it owns no timer and starts no goroutine — so
// the caller decides when to sleep or reschedule.
package refresh
