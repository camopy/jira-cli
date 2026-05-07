package contract

// Envelope Invariants.
//
// Under any input, exit path, or error class, --json output on stdout must:
//   - be valid JSON (jq . succeeds)
//   - contain meta, data, errors, warnings keys
//
// The envelope is required on every JSON-mode response.
// Failures ALSO emit clog diagnostics on stderr (both can coexist).
//
// These tests drive the fix that makes writeCommandError envelope-aware.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireEnvelopeOnStdout runs bin with the given args, captures stdout
// separately from stderr, and asserts the stdout is a valid JSON envelope
// with meta/data/errors/warnings keys.  It returns the decoded envelope map
// and the exit error (nil on success) so callers can make further assertions.
func requireEnvelopeOnStdout(t *testing.T, bin string, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.Bytes()
	if len(out) == 0 {
		t.Fatalf("stdout is empty (stderr: %s); want JSON envelope for args %v",
			stderr.String(), args)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s\nstderr=%s\nargs=%v",
			err, out, stderr.Bytes(), args)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing %q key\nstdout=%s\nargs=%v", key, out, args)
		}
	}
	return env, runErr
}

// requireEnvelopeOnStdoutWithEnv runs bin with the given args and extra env vars,
// otherwise behaves the same as requireEnvelopeOnStdout.
func requireEnvelopeOnStdoutWithEnv(t *testing.T, bin string, extraEnv []string, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.Bytes()
	if len(out) == 0 {
		t.Fatalf("stdout is empty (stderr: %s); want JSON envelope for args %v (env: %v)",
			stderr.String(), args, extraEnv)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s\nstderr=%s\nargs=%v",
			err, out, stderr.Bytes(), args)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing %q key\nstdout=%s\nargs=%v", key, out, args)
		}
	}
	return env, runErr
}

// htmlServer spins up an httptest server that returns an HTML body on every
// request — simulating a maintenance page or misconfigured reverse proxy.
func htmlServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>maintenance</body></html>"))
	}))
}

// assertExitCode asserts that err is an *exec.ExitError with the given exit
// code.  The context string is included in every failure message so that
// callers can annotate known-wrong codes (e.g.  triage) with an
// explanation that surfaces directly in the test output.
func assertExitCode(t *testing.T, err error, want int, context string) {
	t.Helper()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError, got %v (%s)", err, context)
	}
	if ee.ExitCode() != want {
		t.Fatalf("exit code = %d, want %d (%s)", ee.ExitCode(), want, context)
	}
}

// assertValidationExitCode asserts that runErr is an *exec.ExitError with
// exit code 3 (validation errors).
func assertValidationExitCode(t *testing.T, runErr error) {
	t.Helper()
	assertExitCode(t, runErr, 3, "validation errors → exit 3")
}

// TestI1MutexFlagsJsonEnvelope — Attack 3: --json --plain are mutually exclusive.
// Cobra fires the error before RunE, but stdout must still carry a JSON envelope.
// validation errors → exit 3.
func TestI1MutexFlagsJsonEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	env, runErr := requireEnvelopeOnStdout(t, bin, "--json", "--plain", "schema")
	if runErr == nil {
		t.Fatal("expected non-zero exit for mutually exclusive flags, got 0")
	}
	assertValidationExitCode(t, runErr)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want at least one error entry\nenvelope=%v", env)
	}
}

// TestI1MalformedJsonInputEnvelope — Attack 4: --json-input with invalid JSON.
// The parse error must surface as a JSON envelope on stdout, not bare clog text.
// validation errors → exit 3.
func TestI1MalformedJsonInputEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json {["), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	env, runErr := requireEnvelopeOnStdout(t, bin, "issue", "create",
		"--json-input", badJSON, "--json")
	if runErr == nil {
		t.Fatal("expected non-zero exit for malformed JSON, got 0")
	}
	assertValidationExitCode(t, runErr)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want parse error entry\nenvelope=%v", env)
	}
}

// TestI1HugeJsonInputEnvelope — Attack 5: large invalid --json-input file.
// Must return a JSON envelope with an error (not hang or emit bare text).
// validation errors → exit 3.
func TestI1HugeJsonInputEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	// 2 MB is sufficient to exercise the size path without making the test slow.
	hugeJSON := filepath.Join(t.TempDir(), "huge.json")
	data := bytes.Repeat([]byte("{}"), 1024*1024) // 2 MB of "{}"
	// Corrupt the first bytes so it parses as invalid JSON at the object level.
	data[0] = '['
	if err := os.WriteFile(hugeJSON, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	env, runErr := requireEnvelopeOnStdout(t, bin, "issue", "create",
		"--json-input", hugeJSON, "--json")
	if runErr == nil {
		t.Fatal("expected non-zero exit for huge/invalid JSON, got 0")
	}
	assertValidationExitCode(t, runErr)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want error entry\nenvelope=%v", env)
	}
}

// TestI1StdinAndJsonInputEnvelope — Attack 6: both stdin pipe and --json-input supplied.
// --json-input wins (file takes precedence), but stdout must be a JSON envelope.
// validation errors → exit 3.
func TestI1StdinAndJsonInputEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json {["), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(bin, "issue", "create", "--json-input", badJSON, "--json")
	cmd.Stdin = bytes.NewBufferString(`{"summary":"a"}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	assertValidationExitCode(t, runErr)

	out := stdout.Bytes()
	if len(out) == 0 {
		t.Fatalf("stdout is empty (stderr: %s); want JSON envelope", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout=%s", err, out)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing %q\nstdout=%s", key, out)
		}
	}
}

// TestI1HTMLServerResponseEnvelope — Attack 7: server returns HTML (non-JSON body).
// The parse error must surface as a JSON envelope on stdout.
// Server errors → exit 5.
func TestI1HTMLServerResponseEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	srv := htmlServer(t)
	defer srv.Close()

	env, runErr := requireEnvelopeOnStdout(t, bin, "--config", jiraConfig(t, srv.URL),
		"--json", "issue", "list")
	if runErr == nil {
		t.Fatal("expected non-zero exit for HTML server response, got 0")
	}
	// outputErrorFor now type-asserts on *jira.APIError; the client wraps JSON
	// parse failures (e.g. HTML body) as ErrorTypeServer so they reach exit 5.
	assertExitCode(t, runErr, 5, "server-shaped HTML response must exit 5")
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want server-error entry\nenvelope=%v", env)
	}
}

// TestI1AgentDetectedModeEmitsEnvelopeOnError — I2 (review): agent env-var sessions.
// When CLAUDE_CODE=1 is set the detector defaults to ModeCompact even without --json
// or --compact flags. An early error (e.g. malformed --json-input) must still emit a
// parseable envelope to stdout. Previously jsonEnvelopeRequested() only checked
// PersistentFlags, missing the agent-mode path entirely.
// validation errors → exit 3.
func TestI1AgentDetectedModeEmitsEnvelopeOnError(t *testing.T) {
	bin := buildJiraBinary(t)
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json {["), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Set CLAUDE_CODE=1 — triggers agent detection → ModeCompact, no explicit flag.
	env, runErr := requireEnvelopeOnStdoutWithEnv(t, bin,
		[]string{"CLAUDE_CODE=1"},
		"issue", "create", "--json-input", badJSON,
	)
	if runErr == nil {
		t.Fatal("expected non-zero exit for malformed JSON in agent mode, got 0")
	}
	assertValidationExitCode(t, runErr)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty; want parse error entry\nenvelope=%v", env)
	}
}
