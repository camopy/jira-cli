package config

import "testing"

// keyringServiceName honors the JIRA_KEYRING_SERVICE override so the
// end-to-end test suites can confine credential operations to a throwaway
// namespace, and falls back to the default service for any blank or whitespace
// value so production (which never sets it) is unaffected. This lives in an
// untagged file — unlike keyring_test.go, which is gated to the cgo/1Password
// build — because the resolver is pure string logic with no keyring backend
// dependency and should be covered on every build.
func TestKeyringServiceName(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		want string
	}{
		{name: "blank falls back to default", env: "", want: defaultKeyringService},
		{name: "whitespace falls back to default", env: "   ", want: defaultKeyringService},
		{name: "override is honored", env: "jira-cli-test-ns", want: "jira-cli-test-ns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(keyringServiceEnv, tc.env)
			if got := keyringServiceName(); got != tc.want {
				t.Fatalf("keyringServiceName() = %q, want %q", got, tc.want)
			}
		})
	}
}
