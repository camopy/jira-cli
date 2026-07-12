// Package boards hosts the `jira boards` command tree. It exposes a single
// read-only sub-command today, `jira boards list`. The cache primer lives in
// the cache package (`jira cache boards`); this package renders the cached list
// and primes transparently when the cache is empty, so first-run needs no extra
// step.
package boards
