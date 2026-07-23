// Package schema builds the machine-readable command and output schemas that
// `jira agent schema` emits and the docs generator reads. It walks the live
// cobra tree and derives the published error code enum from errtax, so command
// and error schemas cannot drift from their implementations.
package schema
