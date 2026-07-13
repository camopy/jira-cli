package contract

import (
	"os"
	"testing"
)

// TestMain isolates the contract suite from the developer's real OS keyring.
// Every test here drives the actual jira binary; when a fixture uses
// secret_backend = "keyring", the child process sees
// JIRA_TEST_CREDENTIAL_STORE_DIR and uses a temp file-backed credential store
// instead of Secret Service / Keychain / Credential Manager. The keyring
// service override remains as a harmless fallback namespace if a test bypasses
// cmdutil.CredentialStoreFor, but normal contract tests should not touch the
// OS keyring at all.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jira-cli-contract-credentials-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("JIRA_TEST_CREDENTIAL_STORE_DIR", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-contract-test"); err != nil {
		panic(err)
	}
	// Isolate the on-disk metadata cache too: commands now record recently
	// used issue keys as a side effect, and the exec'd binary inherits this
	// process's environment — without the override every test run would
	// write into the developer's real cache root.
	cacheDir, err := os.MkdirTemp("", "jira-cli-test-cache-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CACHE_HOME", cacheDir); err != nil {
		panic(err)
	}
	// And the config file: tests that pass no --config resolve the default
	// path, so the developer's real ~/.config/jira-cli/config.toml — which
	// may hold keys this branch's strict decode does not know (a config
	// written by a newer binary) — would otherwise fail the whole suite.
	cfgDir, err := os.MkdirTemp("", "jira-cli-test-config-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", cfgDir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	_ = os.RemoveAll(cacheDir)
	_ = os.RemoveAll(cfgDir)
	os.Exit(code)
}
