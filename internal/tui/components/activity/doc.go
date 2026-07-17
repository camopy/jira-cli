// Package activity is the observational record of user-facing mutations for
// the `jira tui` dashboard: it feeds the footer's status slot (the most-recent
// operation and an in-flight count) and the scrollable operation-log overlay
// (every recorded mutation, newest-first). It is deliberately observational —
// it records what a mutation is doing and how it resolved, but it never gates
// a write. Callers keep their own re-entrancy guards; the registry only
// watches. Every method runs on the single Bubble Tea Update goroutine, so the
// registry needs no locking (mirroring internal/tui/components/task); it owns
// no goroutines of its own.
package activity
