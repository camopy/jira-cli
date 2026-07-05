// `worklog add --started` normalizes its value to the strict form Jira's
// worklog API requires (yyyy-MM-dd'T'HH:mm:ss.SSS±HHMM) and validates it
// locally, so `--dry-run` previews the exact wire timestamp and an
// unparseable value fails at dry-run (exit 3) instead of on submit — the
// preview must never greenlight input Jira would reject.
package contract

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func worklogAddDryRunStarted(t *testing.T, bin, cfg, started string) string {
	t.Helper()
	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"worklog", "add", "PROJ-1", "--time-spent", "1h", "--started", started,
		"--dry-run", "--no-input").CombinedOutput()
	if err != nil {
		t.Fatalf("worklog add --started %q error = %v\n%s", started, err, out)
	}
	var env struct {
		Data struct {
			Worklog struct {
				Started string `json:"started"`
			} `json:"worklog"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	return env.Data.Worklog.Started
}

func TestWorklogStartedIsNormalizedInDryRun(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	// An explicit-offset value has one correct normalization regardless of
	// the host timezone, so it pins the exact wire form.
	if got, want := worklogAddDryRunStarted(t, bin, cfg, "2026-06-26T10:00:00Z"), "2026-06-26T10:00:00.000+0000"; got != want {
		t.Fatalf("started = %q, want %q", got, want)
	}

	// A relative value resolves against the wall clock, so pin the layout:
	// it must parse under Jira's strict form and carry a numeric offset.
	got := worklogAddDryRunStarted(t, bin, cfg, "2h ago")
	if _, err := time.Parse("2006-01-02T15:04:05.000-0700", got); err != nil {
		t.Fatalf("relative started = %q does not match Jira's layout: %v", got, err)
	}
}

func TestWorklogStartedUnparseableFailsAtDryRun(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"worklog", "add", "PROJ-1", "--time-spent", "1h", "--started", "not-a-time",
		"--dry-run", "--no-input").CombinedOutput()
	if err == nil {
		t.Fatalf("worklog add --started not-a-time --dry-run succeeded; the preview greenlit a value Jira rejects:\n%s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("worklog add --started not-a-time exit = %v, want validation exit 3\n%s", err, out)
	}
	if !strings.Contains(string(out), "--started") {
		t.Fatalf("validation error should name --started; got:\n%s", out)
	}
}
