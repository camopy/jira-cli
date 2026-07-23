// Package issue hosts the `jira issue` command group — the CLI's largest verb —
// covering the issue lifecycle (create, view, list, edit, clone, move, delete,
// transition) and its sub-resources: comments, attachments, links, watchers,
// and rank. Issue-scoped reads carry a structured issue identity; mutation
// outputs retain validated request context across dry/live and single/keyed
// paths while keeping Jira results and readbacks conditional. It also provides
// the top-level `jira open` shortcut.
package issue
