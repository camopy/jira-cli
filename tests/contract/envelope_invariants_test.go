package contract

// Envelope Invariants.
//
// Under any input, exit path, or error class, --json output must:
//   - be valid JSON (jq . succeeds)
//   - contain meta, data, errors, warnings keys
//
// Successful envelopes are written to stdout. Error envelopes are written to
// stderr so stdout stays reserved for successful command output.
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

func TestErrorEnvelopeUsesStderrAndLeavesStdoutEmpty(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "completions", "fish")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("unknown command succeeded; want command_unknown failure")
	}
	assertValidationExitCode(t, err)
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty success channel on error", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`"ok":false`)) {
		t.Fatalf("stderr does not contain a structured error envelope:\n%s", stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`"code":"command_unknown"`)) {
		t.Fatalf("stderr envelope does not carry command_unknown:\n%s", stderr.String())
	}
}

// requireEnvelopeOnStdout runs bin with the given args, captures stdout
// separately from stderr, and asserts a valid JSON envelope with
// meta/data/errors/warnings keys. Successful runs emit the envelope on stdout;
// failing runs emit it on stderr and leave stdout empty. It returns the decoded
// envelope map and the exit error (nil on success) so callers can make further
// assertions.
func requireEnvelopeOnStdout(t *testing.T, bin string, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	return requireEnvelopeFromRun(t, stdout.Bytes(), stderr.Bytes(), runErr, args, nil), runErr
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

	return requireEnvelopeFromRun(t, stdout.Bytes(), stderr.Bytes(), runErr, args, extraEnv), runErr
}

func requireEnvelopeFromRun(t *testing.T, stdout, stderr []byte, runErr error, args, extraEnv []string) map[string]any {
	t.Helper()

	streamName := "stdout"
	stream := stdout
	if runErr != nil {
		if len(bytes.TrimSpace(stdout)) != 0 {
			t.Fatalf("stdout is not empty on error\nstdout=%s\nstderr=%s\nargs=%v",
				stdout, stderr, args)
		}
		streamName = "stderr"
		stream = stderr
	}

	env := decodeJSONEnvelopeFromStream(t, stream, streamName, stdout, stderr, args, extraEnv)
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing %q key\n%s=%s\nargs=%v", key, streamName, stream, args)
		}
	}
	return env
}

func decodeJSONEnvelopeFromStream(t *testing.T, stream []byte, streamName string, stdout, stderr []byte, args, extraEnv []string) map[string]any {
	t.Helper()

	var env map[string]any
	decodeJSONValueFromStream(t, stream, streamName, stdout, stderr, args, extraEnv, &env)
	return env
}

func decodeErrorEnvelopeFromStderr(t *testing.T, stdout, stderr []byte, args []string, target any) {
	t.Helper()
	if len(bytes.TrimSpace(stdout)) != 0 {
		t.Fatalf("stdout is not empty on error\nstdout=%s\nstderr=%s\nargs=%v", stdout, stderr, args)
	}
	decodeJSONValueFromStream(t, stderr, "stderr", stdout, stderr, args, nil, target)
}

func runCommandExpectErrorEnvelope(t *testing.T, cmd *exec.Cmd, target any) ([]byte, []byte, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr == nil {
		t.Fatalf("command succeeded; want error\nstdout=%s\nstderr=%s\nargs=%v",
			stdout.Bytes(), stderr.Bytes(), cmd.Args)
	}
	decodeErrorEnvelopeFromStderr(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, target)
	return stdout.Bytes(), stderr.Bytes(), runErr
}

func decodeJSONValueFromStream(t *testing.T, stream []byte, streamName string, stdout, stderr []byte, args, extraEnv []string, target any) {
	t.Helper()

	line := jsonEnvelopeLineFromStream(t, stream, streamName, stdout, stderr, args, extraEnv)
	if err := json.Unmarshal(line, target); err != nil {
		t.Fatalf("%s JSON envelope is invalid: %v\n%s=%s\nstdout=%s\nstderr=%s\nargs=%v\nenv=%v",
			streamName, err, streamName, line, stdout, stderr, args, extraEnv)
	}
}

func jsonEnvelopeLineFromStream(t *testing.T, stream []byte, streamName string, stdout, stderr []byte, args, extraEnv []string) []byte {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(stream), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		return line
	}
	t.Fatalf("%s has no JSON envelope\n%s=%s\nstdout=%s\nstderr=%s\nargs=%v\nenv=%v",
		streamName, streamName, stream, stdout, stderr, args, extraEnv)
	return nil
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

// TestI1RemovedOutputFlagFails — Attack 3: a removed legacy output flag
// must fail as an unknown flag, never be silently re-aliased onto a mode.
func TestI1RemovedOutputFlagFails(t *testing.T) {
	bin := buildJiraBinary(t)
	if err := exec.Command(bin, "--plain", "agent", "schema").Run(); err == nil {
		t.Fatal("removed flag --plain was accepted; want unknown-flag error")
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
		"--json-input", badJSON, "--output=json")
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
		"--json-input", hugeJSON, "--output=json")
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
// --json-input wins (file takes precedence), and stderr must carry an error envelope.
// validation errors → exit 3.
func TestI1StdinAndJsonInputEnvelope(t *testing.T) {
	bin := buildJiraBinary(t)
	badJSON := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json {["), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := exec.Command(bin, "issue", "create", "--json-input", badJSON, "--output=json")
	cmd.Stdin = bytes.NewBufferString(`{"summary":"a"}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	assertValidationExitCode(t, runErr)

	var env map[string]any
	decodeErrorEnvelopeFromStderr(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing %q\nstderr=%s", key, stderr.String())
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
		"--output=json", "issue", "list")
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
