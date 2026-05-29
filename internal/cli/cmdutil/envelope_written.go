package cmdutil

// EnvelopeWrittenError wraps an error a RunE returns to signal that it already
// emitted a structured error envelope, so the central error writer must not
// double-write. The inner error is still unwrapped for exit-code
// classification.
type EnvelopeWrittenError struct{ Inner error }

func (e EnvelopeWrittenError) Error() string { return e.Inner.Error() }
func (e EnvelopeWrittenError) Unwrap() error { return e.Inner }

// EnvelopeWritten wraps err so a RunE can tell the central error writer it
// already emitted a structured error envelope.
func EnvelopeWritten(err error) error { return EnvelopeWrittenError{Inner: err} }

// DiagnosticWrittenError wraps an error a RunE returns after it has already
// emitted the terminal-facing diagnostic for that failure. The inner error
// remains available for exit-code classification.
type DiagnosticWrittenError struct{ Inner error }

func (e DiagnosticWrittenError) Error() string { return e.Inner.Error() }
func (e DiagnosticWrittenError) Unwrap() error { return e.Inner }

// DiagnosticWritten wraps err so the central error writer does not log the
// same failure a second time.
func DiagnosticWritten(err error) error { return DiagnosticWrittenError{Inner: err} }
