package contract

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jqConfig writes a minimal config so --jq tests run hermetically without
// a Jira server: release-notes reads the embedded changelog, and the
// error-branch test fails locally on key validation before any transport.
func jqConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	body := `
default_profile = "default"

[[profiles]]
name = "default"
base_url = ""
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	return cfg
}

func runJQ(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(buildJiraBinary(t), append([]string{"--config", jqConfig(t)}, args...)...)
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=test-token")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	exit := &exec.ExitError{}
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return stdout.String(), stderr.String(), code
}

func TestJQStringResultsPrintRawAndImplyJSON(t *testing.T) {
	// No --output: --jq implies json even though this suite's environment
	// carries agent markers that would otherwise resolve auto to compact —
	// .meta exists only on the envelope, so the value proves the mode.
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--jq", ".meta.command")
	if code != 0 || stdout != "release.notes\n" {
		t.Fatalf("string result = %q (exit %d), want raw release.notes line", stdout, code)
	}
}

func TestJQNonStringResultsPrintAsJSONPerLine(t *testing.T) {
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--jq", "{c: .meta.command}, .ok")
	if code != 0 || stdout != "{\"c\":\"release.notes\"}\ntrue\n" {
		t.Fatalf("results = %q (exit %d), want one JSON value per line", stdout, code)
	}
}

func TestJQExplicitHumanOutputIsValidationError(t *testing.T) {
	stdout, stderr, code := runJQ(t, "release-notes", "--latest", "--output=human", "--jq", ".ok")
	if code != 3 {
		t.Fatalf("exit = %d, want 3\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--output=human") {
		t.Fatalf("stderr missing conflict diagnosis:\n%s", stderr)
	}
}

func TestJQBadExpressionFailsFastWithStableCode(t *testing.T) {
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--jq", ".data | foo(")
	if code != 3 {
		t.Fatalf("exit = %d, want 3\nstdout=%s", code, stdout)
	}
	var env struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || len(env.Errors) == 0 {
		t.Fatalf("stdout is not an error envelope: %v\n%s", err, stdout)
	}
	if env.Errors[0].Code != "jq_expression_invalid" {
		t.Fatalf("code = %s, want jq_expression_invalid", env.Errors[0].Code)
	}
}

func TestJQRuntimeFailureReportsUnfilteredEnvelope(t *testing.T) {
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--jq", `error("boom")`)
	if code != 3 {
		t.Fatalf("exit = %d, want 3\nstdout=%s", code, stdout)
	}
	var env struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || len(env.Errors) == 0 {
		t.Fatalf("stdout is not an error envelope: %v\n%s", err, stdout)
	}
	if env.Errors[0].Code != "jq_eval_failed" {
		t.Fatalf("code = %s, want jq_eval_failed", env.Errors[0].Code)
	}
}

func TestJQFiltersFailureEnvelopesWithExitPreserved(t *testing.T) {
	stdout, _, code := runJQ(t, "issue", "view", "NOPE", "--jq", ".errors[0].code")
	if code != 3 {
		t.Fatalf("exit = %d, want the command's own validation exit preserved\nstdout=%s", code, stdout)
	}
	if stdout != "validation_failed\n" {
		t.Fatalf("filtered failure envelope = %q, want the bare error code line", stdout)
	}
}

func TestJQOverCompactFiltersTheDataDocument(t *testing.T) {
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--output=compact", "--jq", ".releases | type")
	if code != 0 || stdout != "array\n" {
		t.Fatalf("compact+jq = %q (exit %d), want the data-document field", stdout, code)
	}
}

func TestJQPartialResultsBeforeRuntimeErrorAreDiscarded(t *testing.T) {
	// gojq is lazy: an expression that emits then errors must not leave the
	// emitted lines ahead of the error envelope on stdout.
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--jq", `.ok, error("boom")`)
	if code != 3 {
		t.Fatalf("exit = %d, want 3\nstdout=%s", code, stdout)
	}
	if strings.HasPrefix(stdout, "true") {
		t.Fatalf("partial filtered output leaked ahead of the error envelope:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"jq_eval_failed"`) {
		t.Fatalf("stdout is not the jq_eval_failed envelope:\n%s", stdout)
	}
}

func TestJQInfiniteExpressionIsBoundedByTimeout(t *testing.T) {
	// Without RunWithContext an infinite expression ignores --timeout and
	// survives SIGTERM; the deadline must end it with the timeout exit code.
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--timeout", "1s", "--jq", "repeat(1)|empty")
	if code != 7 {
		t.Fatalf("exit = %d, want 7 (timeout)\nstdout=%s", code, stdout)
	}
}

func TestJQConflictsWithTSVAndInteractive(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"tsv", []string{"issue", "list", "--tsv", "--jq", ".ok"}},
		{"interactive", []string{"release-notes", "--latest", "-i", "--jq", ".ok"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runJQ(t, tc.args...)
			if code != 3 {
				t.Fatalf("exit = %d, want 3\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr, "jq_output_conflict") && !strings.Contains(stdout+stderr, "cannot combine") {
				t.Fatalf("missing conflict diagnosis:\nstdout=%s\nstderr=%s", stdout, stderr)
			}
		})
	}
}

func TestJQEmptyExpressionIsTreatedAsAbsent(t *testing.T) {
	stdout, _, code := runJQ(t, "release-notes", "--latest", "--output=json", "--jq", "")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var env struct {
		OK *bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || env.OK == nil || !*env.OK {
		t.Fatalf("empty --jq did not print the unfiltered envelope: %v\n%s", err, stdout)
	}
}
