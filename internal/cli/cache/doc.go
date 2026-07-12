// Package cache hosts the `jira cache` command group: per-resource primers that
// fetch Jira metadata into the per-profile cache, plus housekeeping (clear,
// refresh). The subcommands are generated from the resource registry
// (internal/cli/cache/registry), the single source of truth.
package cache
