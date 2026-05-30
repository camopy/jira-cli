package auth

import "testing"

// validateEmailField trims then validates with net/mail, so a blank, malformed,
// or whitespace-padded address is rejected/normalised before it is stored and
// silently breaks Jira user lookups.
func TestValidateEmailField(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"", false},
		{"   ", false},
		{"user@example.com", true},
		{"  user@example.com  ", true}, // surrounding space is trimmed
		{"not-an-email", false},
		{"missing-domain@", false},
		{"@missing-local.com", false},
	} {
		err := validateEmailField(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("validateEmailField(%q) error = %v, want ok=%v", tc.in, err, tc.ok)
		}
	}
}
