package cli

import "fmt"

// PrimaryError retains a secondary failure without letting it replace the
// command failure that determines the process taxonomy and exit code.
type PrimaryError struct {
	primary   error
	secondary error
}

// PreservePrimaryError combines primaryErr and secondaryErr while keeping
// primaryErr authoritative for MapError. Both remain discoverable through
// errors.Is and errors.As.
func PreservePrimaryError(primaryErr, secondaryErr error) error {
	switch {
	case primaryErr == nil:
		return secondaryErr
	case secondaryErr == nil:
		return primaryErr
	default:
		return &PrimaryError{primary: primaryErr, secondary: secondaryErr}
	}
}

func (e *PrimaryError) Error() string {
	return fmt.Sprintf("%v; handling that error also failed: %v", e.primary, e.secondary)
}

// Unwrap exposes both failures while MapError keeps primary authoritative.
func (e *PrimaryError) Unwrap() []error {
	return []error{e.primary, e.secondary}
}
