// Package changelog embeds jira-cli's own release notes so the binary can show
// its changelog without reaching the network. The per-change fragments under
// .changes/ are batched by changie into one Markdown file per version; those
// files (and the changelog header) are compiled in here, parsed into structured
// releases, and assembled on demand. This is jira-cli's changelog surfaced to
// its users, and is unrelated to the `jira release-notes` view of a Jira
// project's issues.
package changelog
