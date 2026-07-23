package cli

import (
	"errors"
	"io"
	"os"
	"sync"
)

// writeTracker records the first destination failure from output libraries
// whose finalizers do not return write errors. It serializes writes because a
// command-scoped tracker can be shared by concurrent progress renderers.
type writeTracker struct {
	mu  sync.Mutex
	w   io.Writer
	err error
}

// WriteTracker keeps a writer's first destination failure available after
// libraries with void output finalisers have returned. Writer preserves file
// descriptor capabilities used for terminal detection.
type WriteTracker struct {
	tracker *writeTracker
	writer  io.Writer
}

// NewWriteTracker wraps w for a command-scoped sequence of writes.
func NewWriteTracker(w io.Writer) *WriteTracker {
	tracker, writer := newTrackedWriter(w)
	return &WriteTracker{tracker: tracker, writer: writer}
}

// Writer returns the destination wrapper that records failed and short writes.
func (t *WriteTracker) Writer() io.Writer {
	return t.writer
}

// Err returns the recorded destination failure with the stable output
// taxonomy, or nil when every write completed.
func (t *WriteTracker) Err() error {
	if t == nil || t.tracker == nil {
		return nil
	}
	if err := t.tracker.firstError(); err != nil {
		return NewOutputError(err)
	}
	return nil
}

func (t *writeTracker) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.err != nil {
		return 0, t.err
	}
	n, err := t.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		t.err = err
	}
	return n, err
}

func (t *writeTracker) firstError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// trackedWriter preserves file-descriptor behavior for output libraries that
// use it to detect terminals while still recording destination failures.
func newTrackedWriter(w io.Writer) (*writeTracker, io.Writer) {
	tracker := &writeTracker{w: w}
	if _, ok := w.(interface{ Fd() uintptr }); ok {
		return tracker, fdTrackedWriter{writeTracker: tracker}
	}
	return tracker, tracker
}

func withTrackedWriter(w io.Writer, render func(io.Writer) error) error {
	return TrackWrites(w, render)
}

// TrackWrites runs render with a writer that records the first destination
// failure. A destination failure is returned as an OutputError while any
// renderer error remains separately discoverable.
func TrackWrites(w io.Writer, render func(io.Writer) error) error {
	tracker := NewWriteTracker(w)
	renderErr := render(tracker.Writer())
	return errors.Join(renderErr, tracker.Err())
}

type fdTrackedWriter struct {
	*writeTracker
}

func (w fdTrackedWriter) Fd() uintptr {
	return w.w.(interface{ Fd() uintptr }).Fd()
}

func (w fdTrackedWriter) outputFile() *os.File {
	if f, ok := w.w.(*os.File); ok {
		return f
	}
	if carrier, ok := w.w.(interface{ outputFile() *os.File }); ok {
		return carrier.outputFile()
	}
	return nil
}

func (w fdTrackedWriter) Read(p []byte) (int, error) {
	if r, ok := w.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, os.ErrInvalid
}

func (w fdTrackedWriter) Close() error {
	return nil
}
