// Command gen-docs renders the jira command tree into Markdown reference pages
// for the docs site. It is a developer/CI tool, never compiled into the jira
// binary. It mirrors the GitHub CLI's cmd/gen-docs: build the same Cobra tree
// the binary uses, then walk it with internal/docs.
package main
