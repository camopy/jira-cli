package config

import (
	"os"
	"testing"
)

// TestMain clears JIRA_KEYRING_SERVICE so this package's keyring backend tests
// and JIRA_TEST_CREDENTIAL_STORE_DIR so credential-backend tests are
// deterministic regardless of the ambient environment. Keyring tests use an
// in-memory mock and assert against the default service name; file-store tests
// set their own temp directory explicitly with t.Setenv.
func TestMain(m *testing.M) {
	if err := os.Unsetenv(keyringServiceEnv); err != nil {
		panic(err)
	}
	if err := os.Unsetenv(TestCredentialStoreDirEnv); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
