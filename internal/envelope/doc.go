// Package envelope owns each operation's extensible data object and derives
// its published JSON Schema from the same registered Output type. Fixed
// members and nested identities are concrete structs; tenant-defined Jira
// fields, recursive ADF and reviewed polymorphic values form the dynamic
// boundary. Truly shapeless operations register Dynamic with a reason. It sits
// below internal/cli in the import graph: wire-type imports (internal/jira,
// internal/adf) are allowed, but internal/cli is not. The top-level failure
// envelope and local-output taxonomy remain owned by internal/cli and
// internal/errtax.
package envelope
