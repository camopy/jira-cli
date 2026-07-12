// Package jira is the Jira Cloud REST client and its typed services (issues,
// search, boards, comments, links, attachments, worklogs, users, rank). One
// Client is configured at command startup and shared by every service; the
// read-only and dry-run gates are enforced in the transport itself, so no
// service call can route a mutation around them. Errors are typed at their
// source — transport failures become *APIError with the upstream status,
// locally computed misses get their own types — because the CLI's error
// taxonomy maps classes to stable codes and exit codes and must never guess
// from message text. The package stays free of CLI concerns: no cobra, no
// envelope, no errtax.
package jira
