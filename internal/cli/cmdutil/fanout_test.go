package cmdutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gechr/clog"

	"github.com/matcra587/jira-cli/internal/jira"
)

// A dry-run fan-out must narrate the preview, never the mutation: the
// per-key debug lifecycle says "previewing/previewed issue edit" (and
// "failed to preview" on error), with no mutation-tense phrasing left.
func TestFanOutKeysProgressPreviewNarratesPreview(t *testing.T) {
	var buf bytes.Buffer
	prev := clog.IsVerbose()
	clog.SetVerbose(true)
	clog.SetOutput(clog.NewOutput(&buf, clog.ColorNever))
	t.Cleanup(func() {
		clog.SetVerbose(prev)
		clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto))
	})

	_, err := FanOutKeysProgressPreview(context.Background(), "issue.edit", []string{"A-1"}, 1,
		func(context.Context, string) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatalf("FanOutKeysProgressPreview() error = %v", err)
	}
	_, err = FanOutKeysProgressPreview(context.Background(), "issue.delete", []string{"A-2"}, 1,
		func(context.Context, string) (string, error) { return "", errors.New("boom") })
	if err != nil {
		t.Fatalf("FanOutKeysProgressPreview() error = %v", err)
	}

	out := buf.String()
	for _, expected := range []string{
		"previewing issue edit",
		"previewed issue edit",
		"previewing issue delete",
		"failed to preview issue delete",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("preview lifecycle missing %q; got:\n%s", expected, out)
		}
	}
	for _, banned := range []string{"editing issue", "edited issue", "deleting issue", "deleted issue", "failed to edit", "failed to delete"} {
		if strings.Contains(out, banned) {
			t.Fatalf("preview lifecycle used mutation tense %q; got:\n%s", banned, out)
		}
	}
}

// The per-key debug failure lifecycle prints the error text, which embeds
// Jira-supplied messages — the reason field must cross the terminal
// sanitizer so a crafted message cannot inject escapes into stderr.
func TestFanOutKeysProgressDebugFailureReasonIsSanitized(t *testing.T) {
	var buf bytes.Buffer
	prev := clog.IsVerbose()
	clog.SetVerbose(true)
	clog.SetOutput(clog.NewOutput(&buf, clog.ColorNever))
	t.Cleanup(func() {
		clog.SetVerbose(prev)
		clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto))
	})

	// clog.Ctx falls back to the default logger, which SetOutput above
	// redirects into buf — no context seeding needed.
	want := &jira.APIError{StatusCode: 404, Message: "missing\x1b[2Jwiped\x07"}
	results, err := FanOutKeysProgress(context.Background(), "issue.view", []string{"A-1"}, 1,
		func(context.Context, string) (string, error) { return "", want })
	if err != nil {
		t.Fatalf("FanOutKeysProgress() error = %v", err)
	}
	if len(results) != 1 || !errors.Is(results[0].Err, want) {
		t.Fatalf("results = %+v, want the injected APIError per key", results)
	}

	out := buf.String()
	for _, banned := range []string{"\x1b", "\x07"} {
		if strings.Contains(out, banned) {
			t.Fatalf("debug reason leaked control byte %q:\n%q", banned, out)
		}
	}
	if !strings.Contains(out, "missingwiped") {
		t.Fatalf("sanitized reason text mangled; got %q", out)
	}
}

func TestFanOutKeysPreservesInputOrder(t *testing.T) {
	results, err := FanOutKeys(context.Background(), []string{"A-1", "A-2", "A-3"}, 3, func(_ context.Context, key string) (string, error) {
		switch key {
		case "A-1":
			time.Sleep(20 * time.Millisecond)
		case "A-2":
			time.Sleep(5 * time.Millisecond)
		}
		return key + "-value", nil
	})
	if err != nil {
		t.Fatalf("FanOutKeys() error = %v", err)
	}
	got := []string{results[0].Key, results[1].Key, results[2].Key}
	if !reflect.DeepEqual(got, []string{"A-1", "A-2", "A-3"}) {
		t.Fatalf("result keys = %#v, want input order", got)
	}
	if results[0].Value != "A-1-value" || results[1].Value != "A-2-value" || results[2].Value != "A-3-value" {
		t.Fatalf("result values = %#v", results)
	}
}

func TestFanOutKeysNeverExceedsParallelism(t *testing.T) {
	var current atomic.Int32
	var peak atomic.Int32

	_, err := FanOutKeys(context.Background(), []string{"A-1", "A-2", "A-3", "A-4", "A-5"}, 2, func(_ context.Context, key string) (string, error) {
		now := current.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		current.Add(-1)
		return key, nil
	})
	if err != nil {
		t.Fatalf("FanOutKeys() error = %v", err)
	}
	if got := peak.Load(); got > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", got)
	}
	if got := peak.Load(); got < 2 {
		t.Fatalf("peak concurrency = %d, want helper to use available slots", got)
	}
}

func TestFanOutKeysAttemptsEveryKeyWhenOneFails(t *testing.T) {
	wantErr := errors.New("missing issue")
	var calls atomic.Int32

	results, err := FanOutKeys(context.Background(), []string{"A-1", "A-2", "A-3"}, 2, func(_ context.Context, key string) (string, error) {
		calls.Add(1)
		if key == "A-2" {
			return "", wantErr
		}
		return key + "-ok", nil
	})
	if err != nil {
		t.Fatalf("FanOutKeys() error = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	if !errors.Is(results[1].Err, wantErr) {
		t.Fatalf("A-2 error = %v, want %v", results[1].Err, wantErr)
	}
	if results[0].Err != nil || results[2].Err != nil {
		t.Fatalf("unexpected errors in results: %#v", results)
	}
}

func TestFanOutKeysStopsLaunchingOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32

	results, err := FanOutKeys(ctx, []string{"A-1", "A-2", "A-3"}, 1, func(_ context.Context, key string) (string, error) {
		calls.Add(1)
		cancel()
		return key, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FanOutKeys() error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want only the in-flight key to run", calls.Load())
	}
	if results[0].Key != "A-1" {
		t.Fatalf("first result key = %q, want A-1", results[0].Key)
	}
}

func TestFanOutKeysDoesNotWriteToStdoutOrStderr(t *testing.T) {
	stdout, stderr := captureProcessOutput(t, func() {
		_, err := FanOutKeys(context.Background(), []string{"A-1"}, 1, func(_ context.Context, key string) (string, error) {
			return key, nil
		})
		if err != nil {
			t.Fatalf("FanOutKeys() error = %v", err)
		}
	})
	if stdout != "" || stderr != "" {
		t.Fatalf("FanOutKeys wrote stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestFanOutKeysRejectsNilInputs(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		var ctx context.Context
		_, err := FanOutKeys(ctx, []string{"A-1"}, 1, func(context.Context, string) (string, error) {
			return "", nil
		})
		if err == nil || err.Error() != "context must not be nil" {
			t.Fatalf("FanOutKeys() error = %v, want %q", err, "context must not be nil")
		}
	})

	t.Run("nil function", func(t *testing.T) {
		_, err := FanOutKeys[string](context.Background(), []string{"A-1"}, 1, nil)
		if err == nil || err.Error() != "fanout function must not be nil" {
			t.Fatalf("FanOutKeys() error = %v, want %q", err, "fanout function must not be nil")
		}
	})
}

func BenchmarkFanOutKeys(b *testing.B) {
	keys := make([]string, 32)
	for i := range keys {
		keys[i] = fmt.Sprintf("A-%d", i+1)
	}
	ctx := context.Background()

	for _, parallelism := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("parallelism=%d", parallelism), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				results, err := FanOutKeys(ctx, keys, parallelism, func(_ context.Context, key string) (string, error) {
					return key, nil
				})
				if err != nil {
					b.Fatalf("FanOutKeys() error = %v", err)
				}
				if len(results) != len(keys) {
					b.Fatalf("results len = %d, want %d", len(results), len(keys))
				}
			}
		})
	}
}

func TestFanOutKeysProgressMatchesFanOut(t *testing.T) {
	keys := []string{"A-1", "A-2", "A-3", "A-4"}
	run := func(useProgress bool) ([]KeyResult[string], int) {
		var calls atomic.Int64
		fn := func(_ context.Context, key string) (string, error) {
			calls.Add(1)
			if key == "A-3" {
				return "", errors.New("boom")
			}
			return key + "-value", nil
		}
		var (
			res []KeyResult[string]
			err error
		)
		if useProgress {
			res, err = FanOutKeysProgress(context.Background(), "issue.view", keys, 2, fn)
		} else {
			res, err = FanOutKeys(context.Background(), keys, 2, fn)
		}
		if err != nil {
			t.Fatalf("top-level error = %v (useProgress=%v)", err, useProgress)
		}
		return res, int(calls.Load())
	}

	base, baseCalls := run(false)
	prog, progCalls := run(true)
	if baseCalls != len(keys) || progCalls != len(keys) {
		t.Fatalf("fn call counts base=%d prog=%d, want %d", baseCalls, progCalls, len(keys))
	}
	if len(base) != len(prog) {
		t.Fatalf("result lengths differ: base=%d prog=%d", len(base), len(prog))
	}
	for i := range base {
		if base[i].Key != prog[i].Key || base[i].Value != prog[i].Value || (base[i].Err == nil) != (prog[i].Err == nil) {
			t.Fatalf("result %d differs: base=%+v prog=%+v", i, base[i], prog[i])
		}
	}
}

func TestFanOutKeysProgressStaysSilentInNonTTY(t *testing.T) {
	stdout, stderr := captureProcessOutput(t, func() {
		_, err := FanOutKeysProgress(context.Background(), "issue.view", []string{"A-1", "A-2", "A-3"}, 2, func(_ context.Context, key string) (string, error) {
			return key, nil
		})
		if err != nil {
			t.Fatalf("FanOutKeysProgress() error = %v", err)
		}
	})
	if stdout != "" || stderr != "" {
		t.Fatalf("progress bar leaked output in non-TTY: stdout=%q stderr=%q", stdout, stderr)
	}
}

// Duplicate keys are legal in a key expression; every occurrence must get
// its own rendered row and finish exactly once, or the group Wait would
// wedge on an unfinished task.
func TestFanOutKeysProgressHandlesDuplicateKeys(t *testing.T) {
	keys := []string{"A-1", "A-1", "A-2"}
	var calls atomic.Int64
	res, err := FanOutKeysProgress(context.Background(), "issue.view", keys, 2, func(_ context.Context, key string) (string, error) {
		calls.Add(1)
		return key + "-v", nil
	})
	if err != nil {
		t.Fatalf("FanOutKeysProgress() error = %v", err)
	}
	if int(calls.Load()) != len(keys) || len(res) != len(keys) {
		t.Fatalf("calls=%d results=%d, want %d each", calls.Load(), len(res), len(keys))
	}
	for i, key := range keys {
		if res[i].Key != key || res[i].Value != key+"-v" {
			t.Fatalf("result %d = %+v, want key %s", i, res[i], key)
		}
	}
}

// A cancellation that stops the fan-out before every key ran must not
// wedge the render-group Wait — unfinished rows are swept with the
// terminal error and the call returns.
func TestFanOutKeysProgressReturnsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	keys := []string{"A-1", "A-2", "A-3", "A-4", "A-5", "A-6"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := FanOutKeysProgress(ctx, "issue.view", keys, 2, func(ctx context.Context, key string) (string, error) {
			cancel()
			return key, ctx.Err()
		})
		if err == nil {
			t.Errorf("canceled fan-out must surface the context error")
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("FanOutKeysProgress wedged after context cancellation")
	}
}

func TestFanOutKeysProgressDelegatesForTrivialInputs(t *testing.T) {
	res, err := FanOutKeysProgress(context.Background(), "issue.view", []string{"A-1"}, 1, func(_ context.Context, key string) (string, error) {
		return key + "-v", nil
	})
	if err != nil || len(res) != 1 || res[0].Value != "A-1-v" {
		t.Fatalf("single-key delegate: res=%+v err=%v", res, err)
	}

	var nilCtx context.Context
	if _, err := FanOutKeysProgress(nilCtx, "issue.view", []string{"A-1", "A-2"}, 1, func(context.Context, string) (string, error) {
		return "", nil
	}); err == nil || err.Error() != "context must not be nil" {
		t.Fatalf("nil ctx: err = %v, want %q", err, "context must not be nil")
	}

	if _, err := FanOutKeysProgress[string](context.Background(), "issue.view", []string{"A-1", "A-2"}, 1, nil); err == nil || err.Error() != "fanout function must not be nil" {
		t.Fatalf("nil fn: err = %v, want %q", err, "fanout function must not be nil")
	}
}

func captureProcessOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()
	origStdout := os.Stdout
	origStderr := os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()
	os.Stdout = outW
	os.Stderr = errW

	fn()

	_ = outW.Close()
	_ = errW.Close()
	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, outR)
	_, _ = io.Copy(&errBuf, errR)
	_ = outR.Close()
	_ = errR.Close()
	return outBuf.String(), errBuf.String()
}
