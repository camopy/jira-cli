package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorklogAddSchemaDryRunAndInvalidDuration(t *testing.T) {
	cmd := exec.Command(buildJiraBinary(t), "worklog", "add", "PROJ-1", "--time-spent", "45m", "--dry-run", "--no-input")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output is not JSON: %v\n%s", err, out)
	}

	cmd = exec.Command(buildJiraBinary(t), "worklog", "add", "PROJ-1", "--time-spent", "3w", "--no-input")
	if err := cmd.Run(); err == nil {
		t.Fatal("invalid duration succeeded")
	}
}

func TestWorklogAddUsesProfileWorkdaySeconds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = ""
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 36000
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(buildJiraBinary(t), "--config", path, "--output=json", "worklog", "add", "PROJ-1", "--time-spent", "1d", "--dry-run", "--no-input")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Worklog struct {
				TimeSpentSeconds int `json:"time_spent_seconds"`
			} `json:"worklog"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Worklog.TimeSpentSeconds != 36000 {
		t.Fatalf("1d parsed as %d seconds, want profile workday 36000\n%s", env.Data.Worklog.TimeSpentSeconds, out)
	}
}

func TestWorklogAddAcceptsStartedAndJSONInput(t *testing.T) {
	input := filepath.Join(t.TempDir(), "worklog.json")
	if err := os.WriteFile(input, []byte(`{"time_spent":"45m","started":"2026-05-03T09:30:00.000-0400","comment_markdown":"paired on fix"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(buildJiraBinary(t), "--output=json", "worklog", "add", "PROJ-1", "--dry-run", "--no-input", "--json-input", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add json-input error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Worklog struct {
				TimeSpentSeconds int    `json:"time_spent_seconds"`
				Started          string `json:"started"`
			} `json:"worklog"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("worklog add output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Worklog.TimeSpentSeconds != 2700 || env.Data.Worklog.Started != "2026-05-03T09:30:00.000-0400" {
		t.Fatalf("worklog add json-input output = %+v\n%s", env.Data.Worklog, out)
	}
}
