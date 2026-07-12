// Package registry is the single source of truth for the cacheable Jira
// metadata resources: their identity, default freshness window, and fetch.
// It is deliberately free of cobra command wiring so any consumer — the cache
// command group, `jira boards list`, `jira issue link types`, the refresh-all
// runner — can depend on the resource metadata without pulling in the command
// builders.
package registry
