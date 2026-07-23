package cli

import (
	"errors"
	"io"
	"os"
	"testing"
)

var errWriteSentinel = errors.New("write sentinel")

type alwaysFailWriter struct {
	writes int
}

func (w *alwaysFailWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errWriteSentinel
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

type failAfterWriter struct {
	remaining int
	writes    int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.remaining <= 0 {
		return 0, errWriteSentinel
	}
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return len(p), nil
	}
	n := w.remaining
	w.remaining = 0
	return n, errWriteSentinel
}

func TestWriteTrackerPreservesFirstFailure(t *testing.T) {
	dst := &alwaysFailWriter{}
	tracker := &writeTracker{w: dst}

	if _, err := tracker.Write([]byte("first")); !errors.Is(err, errWriteSentinel) {
		t.Fatalf("first Write() error = %v, want sentinel", err)
	}
	if _, err := tracker.Write([]byte("second")); !errors.Is(err, errWriteSentinel) {
		t.Fatalf("second Write() error = %v, want sentinel", err)
	}
	if !errors.Is(tracker.err, errWriteSentinel) {
		t.Fatalf("tracked error = %v, want sentinel", tracker.err)
	}
	if dst.writes != 1 {
		t.Fatalf("destination writes = %d, want 1 after first failure", dst.writes)
	}
}

func TestWriteTrackerNormalizesShortWrite(t *testing.T) {
	tracker := &writeTracker{w: shortWriter{}}
	const payload = "truncated"

	n, err := tracker.Write([]byte(payload))
	if n != len(payload)-1 {
		t.Fatalf("Write() bytes = %d, want %d", n, len(payload)-1)
	}
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() error = %v, want io.ErrShortWrite", err)
	}
	if !errors.Is(tracker.err, io.ErrShortWrite) {
		t.Fatalf("tracked error = %v, want io.ErrShortWrite", tracker.err)
	}
}

func TestWriteTrackerPreservesFailAfterError(t *testing.T) {
	dst := &failAfterWriter{remaining: 4}
	tracker := &writeTracker{w: dst}

	n, err := tracker.Write([]byte("prefix and suffix"))
	if n != 4 {
		t.Fatalf("Write() bytes = %d, want 4", n)
	}
	if !errors.Is(err, errWriteSentinel) {
		t.Fatalf("Write() error = %v, want sentinel", err)
	}
	if !errors.Is(tracker.err, errWriteSentinel) {
		t.Fatalf("tracked error = %v, want sentinel", tracker.err)
	}
}

func TestFDTrackedWriterPreservesOriginalFileCapability(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	_, out := newTrackedWriter(f)
	carrier, ok := out.(interface{ outputFile() *os.File })
	if !ok {
		t.Fatalf("tracked writer type = %T, want output-file capability", out)
	}
	if got := carrier.outputFile(); got != f {
		t.Fatalf("tracked output file = %p, want %p", got, f)
	}

	_, nested := newTrackedWriter(out)
	nestedCarrier, ok := nested.(interface{ outputFile() *os.File })
	if !ok {
		t.Fatalf("nested tracked writer type = %T, want output-file capability", nested)
	}
	if got := nestedCarrier.outputFile(); got != f {
		t.Fatalf("nested tracked output file = %p, want %p", got, f)
	}
}
