package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigInitProfileGetSetMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	cmd := exec.Command(
		"go", "run", "../../cmd/jira",
		"--config", path,
		"--json",
		"config", "init",
		"--no-input",
		"--profile", "default",
		"--base-url", "https://company.atlassian.net",
		"--auth-type", "token",
		"--email", "dev@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config init error = %v\n%s", err, out)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "api-token") || strings.Contains(string(content), "password =") {
		t.Fatalf("config file contains secret material:\n%s", content)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--json", "config", "get", "default_profile")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config get error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("config get output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Key != "default_profile" || env.Data.Value != "default" {
		t.Fatalf("default_profile envelope = %q", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--json", "config", "set", "theme.path", "/tmp/theme.toml")
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("config set error = %v\n%s", err, out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--json", "config", "profile")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config --help error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "default") {
		t.Fatalf("profile output = %q", out)
	}
}
