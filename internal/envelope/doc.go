// Package envelope holds the typed per-operation Output structs that
// envelope `data` payloads are built from, and derives the published JSON
// Schemas from those same types — one declaration, so emission and schema
// cannot diverge. It sits below internal/cli in the import graph: wire-type
// imports (internal/jira, internal/adf) are allowed, internal/cli is NOT —
// cli imports this package (Pagination's canonical home is here, aliased
// there), so an envelope→cli import closes a cycle. The top-level failure
// envelope and its local-output taxonomy remain owned by internal/cli and
// internal/errtax rather than operation data structs.
package envelope
