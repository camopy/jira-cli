package integration

import (
	"os"
	"testing"
)

// TestMain isolates the integration suite from the developer's real OS keyring.
// Its tests exec the real jira binary with secret_backend = "keyring"; today
// they run only read-only/dry-run commands, but a future credential-mutating
// command would otherwise reach the developer's real "jira-cli" credential.
// Setting JIRA_KEYRING_SERVICE (read by internal/config's keyringServiceName)
// redirects every credential read, write, and delete to a dedicated namespace,
// inherited by the exec'd binaries through the process environment. The
// contract suite does the same; the live suite is deliberately NOT isolated
// because it authenticates against a real tenant with the real credential.
func TestMain(m *testing.M) {
	if err := os.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-integration-test"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
