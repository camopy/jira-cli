// Typed errors for the client's transport-layer mutation guards. Both fire
// in Client.Do before any bytes reach the wire, so every command inherits
// them without per-command boilerplate. They are TYPED (not synthetic
// *APIError values) so the error mapper can recognize them with errors.As
// and emit their own stable codes instead of the validation_failed
// catch-all.
package jira

// ReadOnlyError reports a mutation refused because read-only mode is active
// (the JIRA_READ_ONLY env var or the profile's read_only=true).
type ReadOnlyError struct {
	Method string
	Path   string
}

func (e *ReadOnlyError) Error() string {
	return "read-only mode is active (JIRA_READ_ONLY env or profile read_only=true); refusing " + e.Method + " " + e.Path
}

// DryRunBlockedError reports a mutation that reached the transport under
// --dry-run. Dry-run commands stop before submitting; a request getting this
// far is caught by the transport guard, not user error.
type DryRunBlockedError struct {
	Method string
	Path   string
}

func (e *DryRunBlockedError) Error() string {
	return "dry-run is active; refusing to send " + e.Method + " " + e.Path
}
