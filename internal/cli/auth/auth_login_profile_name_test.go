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

// authLoginForm must start the profile-name field empty, showing the resolved
// default only as a placeholder: huh appends keystrokes to a pre-filled value,
// so pre-filling the name would mangle a typed one and could overwrite the wrong
// profile. Building the form clears the bound value; a regression that pre-fills
// it (the original bug) would leave the default here.
func TestAuthLoginFormStartsProfileNameEmpty(t *testing.T) {
	name := "default" // the resolved default the command hands to the form
	baseURL, email, backend := "", "", "keyring"
	op, vault, item, cred := "", "", "", ""
	var confirmed bool

	_ = authLoginForm(true, &name, "default", &baseURL, &email, &backend, &op, &vault, &item, &cred, &confirmed)

	if name != "" {
		t.Fatalf("authLoginForm left the name field pre-filled with %q; it must start empty so a typed name is not appended", name)
	}
}
