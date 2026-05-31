package auth

import "testing"

// A blank profile-name field means "use the resolved default"; a typed value
// wins and is trimmed. This is the post-form resolution the interactive login
// applies so the editable field can start empty (and never append to a
// pre-filled value).
func TestResolveProfileName(t *testing.T) {
	for _, tc := range []struct{ typed, hint, want string }{
		{"", "work", "work"},
		{"   ", "work", "work"},
		{"personal", "work", "personal"},
		{"  personal  ", "work", "personal"},
	} {
		if got := resolveProfileName(tc.typed, tc.hint); got != tc.want {
			t.Errorf("resolveProfileName(%q, %q) = %q, want %q", tc.typed, tc.hint, got, tc.want)
		}
	}
}

// authLoginForm must never write to the profile-name field it is given: it
// binds the caller's value as-is. Two cases pin that:
//   - an empty field stays empty — the builder does NOT pre-fill it (an
//     earlier bug appended a typed name onto a pre-filled default and could
//     overwrite the wrong profile); the empty-start guarantee is upheld
//     by promptAuthLogin passing a fresh empty field, never by an in-builder clear.
//   - a non-empty field is left untouched — building the form has no side
//     effect on caller state (a regression reintroducing `*field = ""` fails here).
func TestAuthLoginFormDoesNotWriteNameField(t *testing.T) {
	for _, start := range []string{"", "sentinel"} {
		nameField := start
		baseURL, email, backend := "", "", "keyring"
		op, vault, item, cred := "", "", "", ""
		var confirmed bool

		form := authLoginForm(true, &nameField, "default", &baseURL, &email, &backend, &op, &vault, &item, &cred, &confirmed)
		if form == nil {
			t.Fatalf("authLoginForm returned nil (start=%q)", start)
		}
		if nameField != start {
			t.Fatalf("authLoginForm wrote the bound name field %q -> %q; it must bind the value as-is with no side effect", start, nameField)
		}
	}
}
