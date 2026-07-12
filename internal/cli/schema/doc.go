// Package schema builds the machine-readable command and output schemas that
// `jira agent schema` emits and the docs generator reads. It walks the live
// cobra tree, so the published schema cannot drift from the real commands.
package schema
