package contract

import (
	"os"
	"testing"
)

// TestMain isolates the contract suite from the developer's real OS keyring.
// Every test here drives the actual jira binary, which stores credentials in
// the system keyring. Without isolation a test that re-points or logs out a
// profile deletes or overwrites the developer's real "jira-cli" credential the
// moment a fixture's host+profile collides with a live one — which is exactly
// how a fixture once silently revoked a working credential on every
// `go test` run. Setting JIRA_KEYRING_SERVICE (read by internal/config's
// keyringServiceName) redirects every credential read, write, and delete to a
// dedicated namespace; the exec'd binaries inherit it through the process
// environment. Entries left under this namespace are harmless — they never
// intersect production credentials, which live under the default "jira-cli"
// service.
func TestMain(m *testing.M) {
	if err := os.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-contract-test"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
