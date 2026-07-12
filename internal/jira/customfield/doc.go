// Package customfield is the Jira custom-field type registry.
//
// Symmetric with pkg/adf — same envelope, same single-source-of-truth
// pattern. Each entry knows how to validate user-supplied input for
// its type and encode it into the JSON Jira's REST API expects.
//
// Registry consumers (CLI command code) MUST go through this package
// instead of branching on field type names directly.
package customfield
