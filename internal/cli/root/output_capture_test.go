package root

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gechr/clib/complete"
	"github.com/gechr/clog"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/runtime"
)

type countingErrorWriter struct {
	err    error
	writes int
}

func (w *countingErrorWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

// TestCommandOutputCaptureUsesInjectedStreams asserts a root command
// built from a runtime with caller-supplied stdout/stderr buffers writes
// command output into those buffers, never to the process os.Stdout. It
// runs `version`, a side-effect-free leaf, and asserts the buffer caught
// the rendered output.
func TestCommandOutputCaptureUsesInjectedStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(&stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}

	root.SetArgs([]string{"version", "--output=json"})
	if _, execErr := root.ExecuteContextC(context.Background()); execErr != nil {
		t.Fatalf("execute version: %v\nstderr=%s", execErr, stderr.String())
	}

	got := stdout.String()
	if got == "" {
		t.Fatal("version produced no output on the injected stdout buffer; root output bypasses the runtime streams")
	}
	if !strings.Contains(got, "\"version\"") {
		t.Fatalf("version output missing version payload on injected stream:\n%s", got)
	}
}

func TestSuccessfulCommandOutputFailureUsesLocalIOTaxonomy(t *testing.T) {
	writeErr := errors.New("stdout closed")
	stdout := &countingErrorWriter{err: writeErr}
	var stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}

	root.SetArgs([]string{"version", "--output=json"})
	_, execErr := root.ExecuteContextC(context.Background())
	if !errors.Is(execErr, writeErr) {
		t.Fatalf("execute version error = %v, want writer failure", execErr)
	}
	var outputErr *cli.OutputError
	if !errors.As(execErr, &outputErr) {
		t.Fatalf("execute version error type = %T, want *cli.OutputError", execErr)
	}
	mapped := cli.MapError(execErr)
	if mapped.Code != "output_write_failed" || mapped.Type != "io" || mapped.Retryable {
		t.Fatalf("mapped output failure = %#v", mapped)
	}
	if got := cli.ExitCode(mapped); got != 8 {
		t.Fatalf("output failure exit = %d, want 8", got)
	}
	if stdout.writes != 1 {
		t.Fatalf("stdout writes = %d, want 1", stdout.writes)
	}
	if stderr.Len() != 0 {
		t.Fatalf("output failure wrote an unexpected diagnostic to stderr: %q", stderr.String())
	}
}

func TestCompletionPreflightWritesCandidatesToRootOutputOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(&stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	originalArgs := os.Args
	os.Args = []string{"jira", "--@complete=cacheresource", "--@shell=fish"}
	t.Cleanup(func() { os.Args = originalArgs })

	handled, err := handleCompletionPreflight(root)
	if err != nil {
		t.Fatalf("handleCompletionPreflight() error = %v", err)
	}
	if !handled {
		t.Fatal("handleCompletionPreflight() handled = false, want true")
	}
	if stdout.Len() == 0 {
		t.Fatal("completion preflight emitted no candidates")
	}
	if stderr.Len() != 0 {
		t.Fatalf("completion preflight polluted stderr: %q", stderr.String())
	}
}

func TestCompletionPreflightReturnsCandidateWriteFailure(t *testing.T) {
	writeErr := errors.New("stdout closed")
	stdout := &countingErrorWriter{err: writeErr}
	var stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	originalArgs := os.Args
	os.Args = []string{"jira", "--@complete=cacheresource", "--@shell=fish"}
	t.Cleanup(func() { os.Args = originalArgs })

	handled, err := handleCompletionPreflight(root)
	if !handled {
		t.Fatal("handleCompletionPreflight() handled = false, want Clib action handled")
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("handleCompletionPreflight() error = %v, want writer failure", err)
	}
	var outputErr *cli.OutputError
	if !errors.As(err, &outputErr) {
		t.Fatalf("handleCompletionPreflight() error type = %T, want *cli.OutputError", err)
	}
	if stdout.writes != 1 {
		t.Fatalf("stdout writes = %d, want one failed candidate write", stdout.writes)
	}
	if stderr.Len() != 0 {
		t.Fatalf("completion preflight polluted stderr: %q", stderr.String())
	}
}

func TestCompletionCommandReturnsScriptWriteFailure(t *testing.T) {
	writeErr := errors.New("stdout closed")
	stdout := &countingErrorWriter{err: writeErr}
	var stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}

	root.SetArgs([]string{"completion", "fish"})
	_, execErr := root.ExecuteContextC(context.Background())
	if !errors.Is(execErr, writeErr) {
		t.Fatalf("execute completion error = %v, want writer failure", execErr)
	}
	var outputErr *cli.OutputError
	if !errors.As(execErr, &outputErr) {
		t.Fatalf("execute completion error type = %T, want *cli.OutputError", execErr)
	}
	if stdout.writes != 1 {
		t.Fatalf("stdout writes = %d, want one failed script write", stdout.writes)
	}
	if stderr.Len() != 0 {
		t.Fatalf("completion script failure polluted stderr: %q", stderr.String())
	}
}

func TestCompletionPreflightClassifiesOnlyPrintDestinationFailures(t *testing.T) {
	root, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}
	gen := completionGenerator(root)
	writeErr := errors.New("stdout closed")

	got := classifyCompletionPreflightError(
		complete.CompletionFlags{PrintCompletion: true, Shell: "fish"},
		gen,
		writeErr,
	)
	var outputErr *cli.OutputError
	if !errors.As(got, &outputErr) || !errors.Is(got, writeErr) {
		t.Fatalf("classified print error = %v, want discoverable *cli.OutputError", got)
	}

	generationErr := errors.New("unsupported shell")
	got = classifyCompletionPreflightError(
		complete.CompletionFlags{PrintCompletion: true, Shell: "no-such-shell"},
		gen,
		generationErr,
	)
	if !errors.Is(got, generationErr) {
		t.Fatalf("classified generation error = %v, want original %v", got, generationErr)
	}

	installErr := errors.New("install failed")
	got = classifyCompletionPreflightError(
		complete.CompletionFlags{
			InstallCompletion: true,
			PrintCompletion:   true,
			Shell:             "fish",
		},
		gen,
		installErr,
	)
	if !errors.Is(got, installErr) {
		t.Fatalf("classified install error = %v, want original %v", got, installErr)
	}
}

// TestParallelRootCommandsDoNotShareOutputState constructs two root
// commands in the same process, each with its own stdout/stderr buffers
// and its own output mode, and executes a command on each. Executing one
// must not write into the other's buffers: per-instance roots own their
// streams, so command output is never shared process-global state.
func TestParallelRootCommandsDoNotShareOutputState(t *testing.T) {
	var outA, errA, outB, errB bytes.Buffer

	rootA, _, err := NewRootCommandForTest(
		runtime.WithStdout(&outA),
		runtime.WithStderr(&errA),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest A: %v", err)
	}
	rootB, _, err := NewRootCommandForTest(
		runtime.WithStdout(&outB),
		runtime.WithStderr(&errB),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest B: %v", err)
	}

	// Run a command on A only.
	rootA.SetArgs([]string{"version", "--output=json"})
	if _, execErr := rootA.ExecuteContextC(context.Background()); execErr != nil {
		t.Fatalf("execute A: %v", execErr)
	}

	if outA.Len() == 0 {
		t.Fatal("root A produced no output on its own stdout buffer")
	}
	if outB.Len() != 0 || errB.Len() != 0 {
		t.Fatalf("executing root A leaked into root B's buffers: stdout=%q stderr=%q", outB.String(), errB.String())
	}

	// Now run a different command on B with a different output mode.
	rootB.SetArgs([]string{"version", "--output=compact"})
	if _, execErr := rootB.ExecuteContextC(context.Background()); execErr != nil {
		t.Fatalf("execute B: %v", execErr)
	}

	if outB.Len() == 0 {
		t.Fatal("root B produced no output on its own stdout buffer")
	}
	// A's buffer must still hold exactly what A wrote — B's run must not
	// have appended to or cleared it.
	if !strings.Contains(outA.String(), "\"version\"") {
		t.Fatalf("root A's captured output was disturbed by root B's execution:\n%s", outA.String())
	}
	// The two roots used different output modes; the envelope shape
	// proves each resolved its own mode independently. A used json (full
	// envelope with "ok"); B used compact (bare data, no "ok" key).
	if !strings.Contains(outA.String(), "\"ok\"") {
		t.Errorf("root A (json mode) did not emit a full envelope:\n%s", outA.String())
	}
	if strings.Contains(outB.String(), "\"ok\"") {
		t.Errorf("root B (compact mode) emitted an envelope; output mode bled from a shared detector:\n%s", outB.String())
	}
}

func TestRootPersistentPreRunStoresConfiguredClogLoggerInContext(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root, _, err := NewRootCommandForTest(
		runtime.WithStdout(&stdout),
		runtime.WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewRootCommandForTest: %v", err)
	}

	root.SetArgs([]string{"--debug", "version", "--output=json"})
	executed, execErr := root.ExecuteContextC(context.Background())
	if execErr != nil {
		t.Fatalf("execute version: %v\nstderr=%s", execErr, stderr.String())
	}

	logger := clog.Ctx(executed.Context())
	if logger == clog.Default {
		t.Fatal("root persistent pre-run stored clog.Default in command context; want a command-scoped logger")
	}

	const probe = "context logger debug probe"
	logger.Debug().Msg(probe)
	if strings.Contains(stdout.String(), probe) {
		t.Fatalf("context debug log wrote to stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), probe) {
		t.Fatalf("context debug log did not write to stderr:\n%s", stderr.String())
	}
}
