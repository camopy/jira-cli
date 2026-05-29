package integration

import (
	"os"
	"testing"
)

// TestMain isolates the integration suite from the developer's real OS
// keyring. Its tests exec the real jira binary with secret_backend = "keyring";
// the child process sees JIRA_TEST_CREDENTIAL_STORE_DIR and uses a temp
// file-backed credential store instead of Secret Service / Keychain /
// Credential Manager. The live suite is deliberately NOT isolated because it
// authenticates against a real tenant with the real credential.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jira-cli-integration-credentials-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("JIRA_TEST_CREDENTIAL_STORE_DIR", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-integration-test"); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
