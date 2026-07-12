// Package cache is a tiny per-profile JSON file store for Jira metadata
// (labels, epics, projects, …) that's cheap to look up — used by the
// `jira cache <resource>` commands and (eventually) by Cobra shell
// completion functions.
//
// The store is intentionally dumb: one JSON file per resource, atomic
// write, read-time freshness check. No locking — concurrent writers
// would race, but in practice each profile is driven by one user shell.
package cache
