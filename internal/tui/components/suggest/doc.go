// Package suggest is a session-scoped, in-memory TTL cache for slow keyed
// lookups behind the TUI create form — issue-type lists per project and the
// assignee/label suggestion queries that would otherwise re-hit Jira on every
// keystroke or reopen. A Get within the TTL window reuses the last good result
// for that key; past expiry it refetches. Values are keyed and cached
// independently, and errors are never cached so a transient failure retries.
//
// This is ephemeral process state, not internal/cache: nothing here touches
// disk, nothing is per-profile, and nothing survives the TUI session. Reach
// for internal/cache when the goal is durable per-profile metadata; reach for
// this when the goal is to stop a single form from asking Jira the same thing
// twice.
package suggest
