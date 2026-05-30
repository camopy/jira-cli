package issue

import (
	"reflect"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// resolveAssigneeField classifies an --assignee value: me/none/account-id are
// ready wire values; an email is returned as a non-empty email for the caller
// to resolve via /user/search once a client exists. Surrounding whitespace on
// an email is trimmed before validation (the trailing-space-breaks-lookups bug).
func TestResolveAssigneeField(t *testing.T) {
	withID := config.Profile{AccountID: "557058:abc"}
	for _, tc := range []struct {
		name      string
		input     string
		profile   config.Profile
		wantSet   bool
		wantEmail string
		wantWire  any
		wantErr   bool
	}{
		{name: "blank is unset", input: "   "},
		{name: "none clears", input: "none", wantSet: true},
		{name: "unassigned clears", input: "UNASSIGNED", wantSet: true},
		{name: "me uses profile account id", input: "me", profile: withID, wantSet: true, wantWire: map[string]string{"accountId": "557058:abc"}},
		{name: "me without account id errors", input: "me", wantErr: true},
		{name: "literal account id", input: "557058:xyz", wantSet: true, wantWire: map[string]string{"accountId": "557058:xyz"}},
		{name: "email is deferred for resolution", input: "user@example.com", wantSet: true, wantEmail: "user@example.com"},
		{name: "email surrounding space is trimmed", input: "  user@example.com  ", wantSet: true, wantEmail: "user@example.com"},
		{name: "display-name email form is rejected", input: "Person <user@example.com>", wantErr: true},
		{name: "malformed at-value is rejected not treated as account id", input: "user@", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, email, set, err := resolveAssigneeField(tc.input, tc.profile)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if set != tc.wantSet {
				t.Fatalf("set = %v, want %v", set, tc.wantSet)
			}
			if email != tc.wantEmail {
				t.Fatalf("email = %q, want %q", email, tc.wantEmail)
			}
			if !reflect.DeepEqual(wire, tc.wantWire) {
				t.Fatalf("wire = %#v, want %#v", wire, tc.wantWire)
			}
		})
	}
}
