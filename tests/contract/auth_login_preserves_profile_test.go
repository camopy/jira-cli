package contract

// : auth login --no-input must MERGE with the existing profile rather
// than wholesale replace it. Only fields explicitly supplied via flags or
// --json-input should change; pre-existing values (email, account_id,
// default_project, read_only, etc.) must survive a partial update.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthLoginNoInputPreservesExistingProfileFields(t *testing.T) {
	// Arrange: a config with a fully populated default profile.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://oldsite.atlassian.net"
  auth_type = "token"
  email = "user@example.com"
  account_id = "abc123"
  default_project = "JCT"
  read_only = false
  secret_backend = "keyring"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
  editor = "vim"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Act: update only the base_url (e.g. after a URL migration).
	cmd := exec.Command(
		buildJiraBinary(t),
		"--config", path,
		"auth", "login",
		"--no-input",
		"--base-url", "https://newhost.atlassian.net",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login error = %v\n%s", err, out)
	}

	// Assert: the config must now contain the new base_url but must
	// NOT have lost any of the pre-existing fields.
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	s := string(content)

	if !strings.Contains(s, `base_url = "https://newhost.atlassian.net"`) {
		t.Errorf("base_url not updated:\n%s", s)
	}
	if !strings.Contains(s, `email = "user@example.com"`) {
		t.Errorf(": email was erased by partial auth login:\n%s", s)
	}
	if !strings.Contains(s, `account_id = "abc123"`) {
		t.Errorf(": account_id was erased by partial auth login:\n%s", s)
	}
	if !strings.Contains(s, `default_project = "JCT"`) {
		t.Errorf(": default_project was erased by partial auth login:\n%s", s)
	}
	if !strings.Contains(s, `editor = "vim"`) {
		t.Errorf(": editor was erased by partial auth login:\n%s", s)
	}
}
